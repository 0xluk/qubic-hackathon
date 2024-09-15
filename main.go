package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ardanlabs/conf"
	"github.com/pkg/errors"
	"github.com/qubic/go-node-connector/types"
	"github.com/qubic/go-schnorrq"
	"github.com/qubic/qubic-http/protobuff"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"net/http"
	"os"
	"time"
)

const prefix = "QUBIC_HACKATHON"

func main() {
	if err := run(); err != nil {
		log.Fatalf(err.Error())
	}
}

func run() error {
	var cfg struct {
		Server struct {
			ReadTimeout     time.Duration `conf:"default:5s"`
			WriteTimeout    time.Duration `conf:"default:5s"`
			ShutdownTimeout time.Duration `conf:"default:5s"`
			HttpHost        string        `conf:"default:0.0.0.0:8000"`
			Mnemonic        string        `conf:"default:qxotemickgexwfmrdniukihtuhwmvotnuwtyzfrqmchrqoljndjnetv"`
			SourceAddr      string        `conf:"default:FDVORCTKJZVEBFYUXRVUHMPXLMADKSQKAOXLEXUASDGNXXGSXDIACIGHPYSF"`
		}
	}

	if err := conf.Parse(os.Args[1:], prefix, &cfg); err != nil {
		switch err {
		case conf.ErrHelpWanted:
			usage, err := conf.Usage(prefix, &cfg)
			if err != nil {
				return errors.Wrap(err, "generating config usage")
			}
			fmt.Println(usage)
			return nil
		case conf.ErrVersionWanted:
			version, err := conf.VersionString(prefix, &cfg)
			if err != nil {
				return errors.Wrap(err, "generating config version")
			}
			fmt.Println(version)
			return nil
		}
		return errors.Wrap(err, "parsing config")
	}

	out, err := conf.String(&cfg)
	if err != nil {
		return errors.Wrap(err, "generating config for output")
	}
	log.Printf("main: Config :\n%v\n", out)

	http.HandleFunc("/send-transfers", func(w http.ResponseWriter, r *http.Request) {
		transfers, err := decodeRequest(r)
		if err != nil {
			log.Printf("error decoding transfers: %s\n", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		res, err := sendSendManyTx(r.Context(), transfers, cfg.Server.SourceAddr, cfg.Server.Mnemonic)
		if err != nil {
			log.Printf("error send many tx: %s\n", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
		}
		encoder := json.NewEncoder(w)
		if err := encoder.Encode(res); err != nil {
			log.Printf("error encoding response: %s\n", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	log.Fatal(http.ListenAndServe(cfg.Server.HttpHost, nil))

	return nil
}

func decodeRequest(req *http.Request) ([]types.SendManyTransfer, error) {
	defer req.Body.Close()
	decoder := json.NewDecoder(req.Body)

	var payload []types.SendManyTransfer
	err := decoder.Decode(&payload)
	if err != nil {
		return nil, errors.Wrap(err, "decoding payload")
	}

	log.Printf("got payload: %+v\n", payload)

	return payload, nil
}

func sendSendManyTx(ctx context.Context, transfers []types.SendManyTransfer, srcAddr, seed string) (*protobuff.BroadcastTransactionResponse, error) {
	grpcClient, err := grpc.NewClient("213.170.135.5:8004", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, errors.Wrap(err, "creating new grpc client")
	}

	grpcLiveClient := protobuff.NewQubicLiveServiceClient(grpcClient)

	var sendManyPayload types.SendManyTransferPayload
	err = sendManyPayload.AddTransfers(transfers)
	if err != nil {
		return nil, errors.Wrap(err, "adding transfers")
	}

	// https://testapi.qubic.org/v1/tick-info
	tickInfoRes, err := grpcLiveClient.GetTickInfo(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting tick info")
	}
	// source id(pk/seed), dest id, amount

	targetTick := tickInfoRes.TickInfo.Tick + 5
	log.Printf("target tick: %d\n", targetTick)

	tx, err := types.NewSendManyTransferTransaction(
		srcAddr,
		targetTick,
		sendManyPayload,
	)
	if err != nil {
		log.Fatalf("got err: %s when creating simple transfer transaction", err.Error())
	}
	fmt.Printf("source pubkey: %s\n", hex.EncodeToString(tx.SourcePublicKey[:]))

	unsignedDigest, err := tx.GetUnsignedDigest()
	if err != nil {
		log.Fatalf("got err: %s when getting unsigned digest local", err.Error())
	}

	subSeed, err := types.GetSubSeed(seed)
	if err != nil {
		log.Fatalf("got err %s when getting subSeed", err.Error())
	}

	sig, err := schnorrq.Sign(subSeed, tx.SourcePublicKey, unsignedDigest)
	if err != nil {
		log.Fatalf("got err: %s when signing", err.Error())
	}
	fmt.Printf("sig: %s\n", hex.EncodeToString(sig[:]))
	tx.Signature = sig

	encodedTx, err := tx.EncodeToBase64()
	if err != nil {
		log.Fatalf("got err: %s when encoding tx to base 64", err.Error())
	}
	fmt.Printf("encodedTx: %s\n", encodedTx)

	id, err := tx.ID()
	if err != nil {
		log.Fatalf("got err: %s when getting tx id", err.Error())
	}

	fmt.Printf("tx id(hash): %s\n", id)

	res, err := grpcLiveClient.BroadcastTransaction(ctx, &protobuff.BroadcastTransactionRequest{EncodedTransaction: encodedTx})
	if err != nil {
		log.Fatalf("got err: %s when broadcasting tx", err.Error())
	}

	fmt.Printf("%+v\n", res)

	return res, nil
}

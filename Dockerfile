FROM golang:1.22 AS builder
ENV CGO_ENABLED=0

WORKDIR /src
COPY . /src

RUN go mod tidy
RUN go build -o "/src/bin/qubic-test"

# We don't need golang to run binaries, just use alpine.
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y ca-certificates
COPY --from=builder /src/bin/qubic-test /app/qubic-test
RUN chmod +x /app/qubic-test

EXPOSE 8000

WORKDIR /app

ENTRYPOINT ["./qubic-test"]

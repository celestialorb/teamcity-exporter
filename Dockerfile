FROM golang:1.19 as builder

WORKDIR /opt/go
COPY go.mod ./
COPY go.sum ./
COPY *.go ./

RUN go mod tidy
RUN CGO_ENABLED=1 GOEXPERIMENT=boringcrypto go build -o exporter *.go

FROM gcr.io/distroless/base-debian11:nonroot

WORKDIR /opt/go
COPY --from=builder /opt/go/exporter /opt/go/exporter
ENTRYPOINT ["/opt/go/exporter"]
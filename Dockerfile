# syntax=docker/dockerfile:1

# ---- build stage -----------------------------------------------------------
# The UI is plain HTML/CSS/JS embedded with go:embed, so no Node toolchain and
# no asset build step are needed.
FROM golang:1.22-alpine AS builder
WORKDIR /src

ENV CGO_ENABLED=0 GOOS=linux

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=0.2.6
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/net-bloop ./cmd/net-bloop

# ---- runtime stage ---------------------------------------------------------
# scratch keeps the image tiny; time/tzdata and the CA bundle are embedded or
# copied in, so DNS over HTTPS / ACME / webhooks work without a shell.
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/net-bloop /net-bloop

ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt \
    TZ=Asia/Shanghai \
    NETBLOOP_CONFIG=/data/net-bloop.toml

WORKDIR /data
VOLUME ["/data"]
# 16601 admin panel, 33220 the single reverse-proxy listener.
EXPOSE 16601 33220

ENTRYPOINT ["/net-bloop"]
CMD ["-config", "/data/net-bloop.toml"]

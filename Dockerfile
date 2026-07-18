# Multi-stage, static binary. Runs on Linux hosts, Docker Desktop (Mac/Windows),
# and any Kubernetes cluster. No shell, no libc dependency.
FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/llm-gateway \
    ./cmd/gateway

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/llm-gateway /llm-gateway
# Sensible default config (override with a volume or ConfigMap).
COPY gateway.example.yaml /config/gateway.yaml
USER nonroot:nonroot
EXPOSE 8787
ENTRYPOINT ["/llm-gateway"]
CMD ["-config", "/config/gateway.yaml"]

# Build stage — Go'nun statik binary derlemesi sayesinde final image'a
# sadece derlenmiş dosyayı taşıyoruz, Go toolchain'inin kendisi final
# image'a hiç girmiyor.
FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/api ./cmd/api

# Final stage — minimal, ~10-20MB
FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=builder /app/bin/api /app/api
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
  CMD curl -f http://localhost:8080/metrics || exit 1
ENTRYPOINT ["/app/api"]

# Ana backend'le aynı iki aşamalı desen: builder'da derle, minimal
# imajda çalıştır. Bağımsız bir Go modülü olduğu için ana uygulamanın
# go.mod'undan tamamen izole — versiyon/derleme etkileşimi yok.
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 go build -o /out/mock-json-provider-a .

FROM alpine:3.20
RUN apk add --no-cache curl
COPY --from=builder /out/mock-json-provider-a /usr/local/bin/mock-json-provider-a
EXPOSE 4001
ENTRYPOINT ["mock-json-provider-a"]

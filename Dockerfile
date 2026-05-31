FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o transbridge ./cmd/transbridge

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/transbridge .
COPY --from=builder /app/config.yml ./config.yml.default
VOLUME /root/config
EXPOSE 8080
CMD ["sh", "-c", "if [ -f /root/config/config.yml ]; then ./transbridge -config /root/config/config.yml; else ./transbridge -config ./config.yml.default; fi"]

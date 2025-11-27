FROM golang:1.23-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/server/main.go

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/main .
COPY web/ ./web/

EXPOSE 8080

CMD ["./main"]
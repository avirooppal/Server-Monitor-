FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o main main.go

FROM alpine:latest

WORKDIR /app

# Install smartmontools for disk stats if needed, though gopsutil uses /proc
RUN apk add --no-cache smartmontools

COPY --from=builder /app/main .

CMD ["./main"]

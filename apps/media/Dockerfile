FROM golang:1.25.5-alpine AS base
RUN apk add --no-cache ffmpeg

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o bin/server main.go

EXPOSE 8080 50000-50050/udp

CMD ["./bin/server"]

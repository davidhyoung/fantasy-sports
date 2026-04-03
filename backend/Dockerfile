FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o bin/api ./cmd/api

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/bin/api .
EXPOSE 8080
CMD ["./api"]

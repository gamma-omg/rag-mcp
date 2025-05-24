FROM golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o rag-mcp -ldflags="-s -w" .


FROM alpine:latest

RUN apk add --no-cache poppler-utils

WORKDIR /

COPY --from=builder /app/rag-mcp /rag-mcp
ENTRYPOINT [ "/rag-mcp" ]
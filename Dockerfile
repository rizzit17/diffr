FROM golang:1.23-alpine AS builder

WORKDIR /app

# Pre-copy go.mod and go.sum for caching
COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/diffr ./cmd/diffr

# Minimal runtime image
FROM alpine:3.20

RUN apk add --no-cache git bash

WORKDIR /app

COPY --from=builder /bin/diffr /usr/local/bin/diffr
COPY web/ ./web/

EXPOSE 8080

ENTRYPOINT ["diffr"]
CMD ["serve"]

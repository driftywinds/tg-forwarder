FROM golang:1.23-alpine AS builder

# sqlite3 needs CGO
RUN apk add --no-cache gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o filestore-bot .

# ── Runtime image ─────────────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /app/filestore-bot .

# Persist the database across restarts
VOLUME ["/app/data"]
ENV DB_PATH=/app/data/filestore.db

CMD ["./filestore-bot"]
# ---------- Build stage ----------
FROM golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/webhookhub ./cmd/webhookhub


# ---------- Final stage ----------
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

WORKDIR /app

COPY --from=builder --chown=nonroot:nonroot /app/webhookhub /app/webhookhub

EXPOSE 8080

ENTRYPOINT ["/app/webhookhub"]

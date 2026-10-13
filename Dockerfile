# ---------- Build stage ----------
FROM golang:1.27.2-alpine3.24@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS builder

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

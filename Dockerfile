# ---------- Build stage ----------
FROM golang:1.23-alpine AS builder

WORKDIR /app


RUN apk add --no-cache git build-base

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=1 GOOS=linux go build -o /app/webhookhub ./cmd/webhookhub
RUN chmod +x /app/webhookhub


# ---------- Final stage ----------
FROM alpine:latest

RUN apk add --no-cache ca-certificates \
  && addgroup -S appgroup \
  && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/webhookhub /app/webhookhub
COPY --from=builder /app/web /app/web

RUN mkdir data  && chown -R appuser:appgroup /app 

USER appuser:appgroup

EXPOSE 8080

ENTRYPOINT ["/app/webhookhub"]

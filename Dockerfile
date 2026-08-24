# ---------- Build stage ----------
FROM golang:1.27-alpine3.24@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/webhookhub ./cmd/webhookhub


# ---------- Final stage ----------
FROM gcr.io/distroless/static-debian13:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6

WORKDIR /app

COPY --from=builder --chown=nonroot:nonroot /app/webhookhub /app/webhookhub
COPY --from=builder --chown=nonroot:nonroot /app/web /app/web

EXPOSE 8080

ENTRYPOINT ["/app/webhookhub"]

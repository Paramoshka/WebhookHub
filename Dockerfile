# ---------- Build stage ----------
FROM golang:1.27rc2-alpine3.24@sha256:dcbb18cc5fa1082364dc6aa95224b6b55429d09cbb9631a053d8064c1c367300 AS builder

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

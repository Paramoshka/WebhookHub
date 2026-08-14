# ---------- Build stage ----------
FROM golang:1.26.6-alpine3.24@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df AS builder

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

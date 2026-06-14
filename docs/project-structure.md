# Project Structure

This document describes the folder structure of the WebhookHub project.

    webhookhub/
    │
    ├── cmd/
    │   └── webhookhub/         # main.go # Entry point for the application
    │
    ├── internal/
    │   ├── forwarder/          # Delivery execution and retry worker
    │   ├── handler/            # HTTP handlers for auth, dashboard, forwarding, DLQ, and webhook routes
    │   ├── hmacsig/            # Incoming/outgoing HMAC signature helpers
    │   ├── model/              # GORM models (Webhook, DeliveryAttempt, ForwardingRule, User)
    │   └── storage/            # Database access helpers and query methods
    │
    ├── web/                    # static assets, templates, frontend (if using built-in UI)
    │   ├── static/
    │   └── templates/
    │
    ├── docs/                   # Supporting project documentation and screenshots
    │
    ├── Dockerfile
    ├── docker-compose.yml      # dev
    ├── go.mod
    ├── go.sum
    └── README.md

## Key Runtime Areas

- `internal/forwarder`
  Handles outbound delivery attempts, retry scheduling, and DB-backed retry worker polling.
- `internal/handler`
  Serves the login flow, dashboard, forwarding rules UI, webhook inspection, and DLQ management UI.
- `internal/storage`
  Owns filtered webhook queries, forwarding rule persistence, delivery metrics, and due-retry claiming.
- `web/templates`
  Contains the server-rendered HTML templates for the dashboard and admin UI.

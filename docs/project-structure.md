# Project Structure

This document describes the folder structure of the WebhookHub project.

    webhookhub/
    │
    ├── cmd/
    │   └── webhookhub/         # main.go # Entry point for the application
    │
    ├── internal/
    │   ├── handler/            # HTTP handlers (webhook receiver, API, UI routes)
    │   ├── forwarder/          # Logic for forwarding webhooks to destination URLs
    │   ├── storage/            # Database interaction layer (db/file)
    │   ├── model/              # Data structures (Webhook, Target, etc.)
    │   ├── config/             # Configuration loading (env vars, flags, etc.)
    │   └── utils/              # Helper functions (signing, retry logic, etc.)
    │
    ├── migrations/             # SQL database schema (Postgres/SQLite)
    │
    ├── web/                    # static assets, templates, frontend (if using built-in UI)
    │   ├── assets/
    │   └── templates/
    │
    ├── api/                    # OpenAPI specification (optional)
    │
    ├── Dockerfile
    ├── docker-compose.yml      # dev
    ├── go.mod
    ├── go.sum
    └── README.md

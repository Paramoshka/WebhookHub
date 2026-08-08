# WebhookHub

📬 **WebhookHub** is a lightweight, self-hosted service for receiving, logging, and forwarding webhooks.

Use it to debug, inspect, replay, and route incoming webhooks during development or in production. No third-party services, no cloud lock-in — just full control.

![WebhookHub Dashboard](docs/screenshots/webhookhub-dashboard.png)

---

## 🚀 Why WebhookHub?

When working with external services (Stripe, GitHub, Telegram, Shopify, etc.), developers often face the same pain points:

- ❓ *Where did the webhook go? Why didn’t my service receive it?*
- 🔁 *How do I replay a webhook for debugging or recovery?*
- 🔍 *How do I inspect payloads and headers easily?*
- 📡 *How can I fan-out one webhook to multiple services?*

WebhookHub provides a simple, developer-friendly solution to these problems.

---

## ✨ Core Features (MVP)

- ✅ Receive webhooks at `/hook/:source`
- ✅ Log full payloads, headers, timestamps
- ✅ Replay any webhook via Web UI
- ✅ Forwarding rule per source
- ✅ Optional incoming/outgoing HMAC signing (Stripe-style header format)
- ✅ Web dashboard with filters, pagination
- ✅ Secure login (admin account)
- ✅ Postgres + GORM backend
- ✅ Dockerized and ready to deploy

---

## 📌 Roadmap

### MVP - v0.1
- [x] Accept and log webhooks
- [x] View logs with filters and pagination
- [x] Replay webhooks on demand
- [x] Add/edit/delete forwarding rules
- [x] Delete individual webhook logs
- [x] Admin auth (session cookie + bcrypt)
- [x] PostgreSQL + GORM backend
- [x] Docker + compose setup

### v0.2+
- [x] HMAC signature verification (e.g., Stripe-style)
- [x] Delivery status tracking + metrics
- [x] Dead-letter queue
- [x] Configurable retry/backoff policy
- [x] Dead-letter queue management UI

### v0.3+
- [x] Advanced search and filters
- [x] Retention / cleanup policies

### v0.3.1 - Production baseline
- [x] Explicit database error handling
- [x] Recovery of interrupted deliveries with database leases
- [x] Bounded delivery worker pool and graceful shutdown
- [x] Request body limits and source/target validation
- [x] Required secure configuration with fail-fast validation
- [x] HTTP method restrictions and CSRF protection
- [x] Liveness and readiness endpoints
- [x] CI and critical unit/integration tests

### v0.3.2+
- [ ] Export and bulk redelivery tools
- [ ] Telegram integration
- [ ] OpenAPI schema

### v0.4+
- [ ] Plugin system for custom processors

---

## 🛠️ Tech Stack

| Component     | Technology        |
|---------------|-------------------|
| Language      | Go                |
| Database      | PostgreSQL (via GORM) |
| UI            | HTML + HTMX       |
| Auth          | SecureCookie + bcrypt |
| Container     | Docker + Compose  |

---

## 📦 Getting Started

```bash
git clone https://github.com/Paramoshka/WebhookHub.git
cd webhookhub
cp .env.sample .env
# Fill every required value in .env before starting the service.
docker-compose up -d --build
```

### 🔐 Generate Session Key

WebhookHub uses a 32-byte secret key to sign session cookies.  
You must set this in your `.env` file as `SESSION_KEY`.

To generate a secure random key:

```bash
openssl rand -hex 32
```

## ⚙️ Configuration

WebhookHub validates configuration at startup and exits if a required value is missing or unsafe.

Required values:

- `ADMIN_EMAIL`
- `ADMIN_PASSWORD` — at least 12 characters
- `SESSION_KEY` — at least 32 characters; generate it with `openssl rand -hex 32`
- `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`

Production deployments behind HTTPS should set `COOKIE_SECURE=true`. PostgreSQL TLS can be configured with `POSTGRES_SSLMODE` (default: `disable`).
`ADMIN_EMAIL` and `ADMIN_PASSWORD` are authoritative bootstrap credentials: on startup WebhookHub creates the single admin or updates that account to match the configured values.

Runtime limits and delivery workers:

```dotenv
PORT=8080
MAX_BODY_BYTES=1048576
DELIVERY_WORKERS=4
DELIVERY_POLL_INTERVAL=1s
DELIVERY_LEASE_DURATION=30s
SHUTDOWN_TIMEOUT=10s
```

Delivery is at-least-once. A database lease lets another worker recover a webhook left in `processing` after a crash. A duplicate remains possible if the target accepted a request but WebhookHub stopped before persisting the result.

Health endpoints:

- `GET /healthz` reports that the process is running.
- `GET /readyz` reports readiness only when PostgreSQL responds.

### Retention cleanup

Retention cleanup is optional and disabled by default. Configure via `.env`:

```dotenv
# Retention cleanup
RETENTION_ENABLED=false
RETENTION_DAYS=30
RETENTION_INTERVAL=24h
RETENTION_BATCH_SIZE=300
```

When enabled, expired webhooks are removed in batches every interval by `received_at`.

### Advanced logs filters (UI and API)

`/dashboard` includes advanced filters in the UI and the same parameters are available via:

`GET /partials/webhooks`

Query params:

- `source`: exact match on webhook source.
- `status`: exact match on status (`pending`, `processing`, `retrying`, `success`, `failed`, `skipped`, `dead_lettered`).
- `q`: full-text search across `source`, payload, headers, last error, and DLQ reason.
- `from`: lower bound for `received_at` (supports `RFC3339`, `RFC3339Nano`, `2006-01-02T15:04`, `2006-01-02`).
- `to`: upper bound for `received_at` (supports the same formats as `from`; date-only values are interpreted as end-of-day).
- `sort`: one of `id_desc` (default), `received_desc`, `received_asc`, `id_asc`.
- `page`: page number (default `1`), 10 items per page.

Example:

```bash
curl "http://localhost:8080/partials/webhooks?source=stripe&status=failed&q=payment&from=2026-07-01&to=2026-07-11T23:59&sort=received_desc&page=2"
```

## 📄 License

This project is licensed under AGPL-3.0 for self-hosted and open-source use.

Commercial SaaS deployment or integration into paid platforms requires a separate license. Contact [ivan.parfenov.42a@gmail.com] for details.

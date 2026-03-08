# WebhookHub

📬 **WebhookHub** is a lightweight, self-hosted service for receiving, logging, and forwarding webhooks.

Use it to debug, inspect, replay, and route incoming webhooks during development or in production. No third-party services, no cloud lock-in — just full control.

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
- ✅ Forwarding rules per source (fan-out, routing)
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
- [ ] Delivery status tracking + metrics
- [ ] Dead-letter queue
- [ ] Ngrok/localtunnel integration (for local dev)
- [ ] OpenAPI schema
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
git clone https://github.com/yourname/webhookhub
cd webhookhub
docker-compose up -d --build
```

### 🔐 Generate Session Key

WebhookHub uses a 32-byte secret key to sign session cookies.  
You must set this in your `.env` file as `SESSION_HASH_KEY`.

To generate a secure random key:

```bash
openssl rand -hex 32
```

```bash
curl -X POST http://localhost:8080/hook/test \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Source: test" \
  -d '{"event":"test.ping","message":"Hello from test curl"}'
```

## 📄 License

This project is licensed under AGPL-3.0 for self-hosted and open-source use.

Commercial SaaS deployment or integration into paid platforms requires a separate license. Contact [ivan.parfenov.42a@gmail.com] for details.

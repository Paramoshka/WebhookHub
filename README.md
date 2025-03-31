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

- ✅ Receive incoming webhooks via a public HTTP endpoint
- ✅ Log all requests with payloads and headers
- ✅ Replay any webhook manually
- ✅ Forward webhooks to one or more configured destinations
- ✅ Basic filtering and routing logic
- ✅ SQLite or Postgres storage backend
- ✅ Web UI for browsing and replaying events
- ✅ Docker support for easy local/dev setup

---

## 📌 Roadmap

### MVP - v0.1
- [x] Accept and log webhooks via `/hook/:source`
- [x] Store payloads, headers, timestamps in DB
- [x] Replay webhooks to configured URLs
- [ ] Retry logic with backoff
- [x] Forwarding rules (fan-out, filtering)
- [x] Web UI (basic log viewer + replay button)
- [ ] Auth for protected endpoints (API keys)

### v0.2+
- [ ] HMAC signature verification (e.g., Stripe-style)
- [ ] Delivery status tracking + metrics
- [ ] Dead-letter queue
- [ ] Ngrok/localtunnel integration (for local dev)
- [ ] OpenAPI schema
- [ ] Plugin system for custom processors

---

## 🛠️ Tech Stack

- **Language:** Go
- **Database:** SQLite (default), Postgres (optional)
- **UI:** HTML templates or optional React SPA
- **Container:** Docker / docker-compose

---

## 📦 Getting Started

```bash
git clone https://github.com/yourname/webhookhub
cd webhookhub
docker-compose up -d --build
```

```bash
curl -X POST http://localhost:8080/hook/test \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Source: test" \
  -d '{"event":"test.ping","message":"Hello from test curl"}'
```

## License

MIT — free to use, modify, and deploy.

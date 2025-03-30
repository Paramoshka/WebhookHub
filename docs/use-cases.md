# 📚 WebhookHub Use Cases

## 1. Local Development

Use WebhookHub as a local receiver for external webhooks (e.g., Stripe, GitHub).

- Receive webhooks on `/hook/:source`
- Log and inspect them in the UI
- Replay to your local dev service

## 2. Centralized Logging in Staging/Prod

- Point all 3rd-party webhooks to WebhookHub
- Use it as a centralized logging and inspection layer
- Forward to internal services (e.g., microservices)

## 3. Replay and Debugging

- Inspect payloads in the UI
- Re-deliver any webhook manually
- Retry failed ones

## 4. Fan-out Webhooks

- One webhook → multiple targets
- Example: `/hook/github` → CI + Notifier + Logger

## 5. Reliability Layer

- Prevent loss of webhooks on service downtime
- Store → retry → forward when target is back
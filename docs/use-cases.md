# 📚 WebhookHub Use Cases

## 1. Local Development

Use WebhookHub as a local receiver for external webhooks (e.g., Stripe, GitHub).

- Receive webhooks on `/hook/:source`
- Log and inspect them in the UI
- Replay to your local dev service
- Use an external tunnel such as ngrok when a third-party service needs a public callback URL

## 2. Centralized Logging in Staging/Prod

- Point all 3rd-party webhooks to WebhookHub
- Use it as a centralized logging and inspection layer
- Forward each source to its configured internal target

## 3. Replay and Debugging

- Inspect payloads in the UI
- Re-deliver any webhook manually
- Review delivery attempts, last error, and target response

## 4. Reliability Layer

- Persist incoming webhooks before delivery
- Retry failed deliveries with configurable attempts and backoff
- Move exhausted deliveries into the dead-letter queue
- Requeue or delete dead-lettered webhooks from the UI

## 5. Operational Visibility

- Track delivery metrics in the dashboard
- Filter recent activity by source and status
- Inspect dead-lettered items separately in `/dlq`

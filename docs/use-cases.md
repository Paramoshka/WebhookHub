# 📚 WebhookHub Use Cases

[Quick start](../README.md#quick-start) · [UI and HTTP reference](reference.md)

## 1. Local Development

Use WebhookHub as a local receiver for external webhooks (e.g., Stripe, GitHub).

- Receive webhooks on `POST /hook/{source}`
- Log and inspect them in the UI
- [Configure a forwarding rule](forwarding-ui.md) to reach your dev service
- Inspect an event and Replay it after fixing your handler
- Use an [external tunnel](ngrok-guide.md) when a third-party service needs a public callback URL

When running in Docker, the forwarding target must be reachable from the
application container. `localhost` in a target URL refers to that container.

## 2. Centralized Logging in Staging/Prod

- Point all 3rd-party webhooks to WebhookHub
- Use it as a centralized logging and inspection layer
- Forward each source to its single configured internal target
- Optionally [clean up old events](operations.md#retention-cleanup)

## 3. Replay and Debugging

- Inspect payloads in the UI
- Queue another delivery when the event is not currently being processed
- Review each delivery attempt, last error, and saved target response
- Download an individual payload as its original bytes

See [Inspect a webhook](reference.md#inspect-a-webhook). Replay keeps attempt
history and uses the current forwarding rule.

## 4. Reliability Layer

- Persist incoming webhooks before delivery
- Retry failed deliveries with configurable attempts and backoff
- Move exhausted deliveries into the dead-letter queue
- After fixing the receiver, requeue individual events or
  [up to 20 selected DLQ events](reference.md#bulk-recovery-from-dlq) at a time
- Delete events that no longer need recovery

Delivery is [at-least-once](operations.md#delivery-behavior). Receivers must
tolerate duplicates; requeuing confirms scheduling, not successful delivery.

## 5. Operational Visibility

- Track delivery metrics in the dashboard
- Filter recent activity by source and status
- Search payload text and errors or narrow the time range with
  [timezone-aware filters](reference.md#advanced-logs-filters-ui-and-api)
- Inspect dead-lettered items separately in `/dlq`

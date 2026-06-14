# 🔁 Configure Forwarding in Web UI

WebhookHub already includes a Web UI for managing forwarding rules.

## What you can configure

- Add, edit, and delete one forwarding rule per source
- Set the target URL for outgoing delivery
- Verify incoming HMAC signatures
- Sign outgoing deliveries
- Configure retry policy per rule
  - max attempts
  - initial backoff in seconds

## Current behavior

- Each source currently maps to a single target URL
- Delivery retries use exponential backoff and eventually move failed webhooks to the DLQ
- You can inspect delivery attempts from the webhook detail page

## Where to find it

- Open `/forwarding` after logging in
- Open `/dashboard` for metrics and recent activity
- Open `/dlq` to manage dead-lettered webhooks

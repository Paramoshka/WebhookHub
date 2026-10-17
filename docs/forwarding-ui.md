# 🔁 Configure Forwarding in Web UI

Open `/forwarding` after logging in to manage forwarding rules. See the
[quick start](../README.md#quick-start) for a first test event.

## What you can configure

- Add, edit, and delete one forwarding rule per source
- Set the target URL for outgoing delivery
- Verify incoming HMAC signatures
- Sign outgoing deliveries
- Configure retry policy per rule
  - max attempts
  - initial backoff in seconds

The target must be an absolute HTTP(S) URL without embedded credentials and must
be reachable from the application. In Docker, `localhost` refers to the
application container; use a Compose service name and container port for another
service in the same network.

Incoming verification and outgoing signing use the
[Stripe-style HMAC format](reference.md#hmac-signatures). The incoming header name
is configurable; the signature format is fixed.

## Current behavior

- Each source currently maps to a single target URL
- Delivery retries default to 3 attempts and a 2-second initial backoff; delays
  double up to a 30-second cap, and exhausted deliveries move to the DLQ
- Only HTTP `2xx` responses count as successful delivery; redirects are not followed
- Events without a forwarding rule are saved and marked `skipped` by the worker;
  adding a rule does not send them automatically, so use Replay
- Replay queues delivery using the current rule and keeps attempt history
- You can [inspect delivery attempts and responses](reference.md#inspect-a-webhook)
  from the webhook detail page

See [delivery behavior](operations.md#delivery-behavior) for duplicate delivery
and active-delivery restrictions.

## Where to find it

- Open `/forwarding` after logging in
- Open `/dashboard` for metrics and recent activity
- Open `/dlq` to manage dead-lettered webhooks, including
  [requeuing up to 20 selected events](reference.md#bulk-recovery-from-dlq)

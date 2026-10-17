# Operations

[Back to README](../README.md) · [UI and HTTP reference](reference.md)

## Configuration

WebhookHub loads `.env` at startup and validates required values before serving
requests. Existing environment variables take precedence over `.env`. Start with
[`.env.sample`](../.env.sample).

| Required variable | Meaning |
| --- | --- |
| `ADMIN_EMAIL` | Valid email address for the single admin account |
| `ADMIN_PASSWORD` | Admin password, at least 12 characters |
| `SESSION_KEY` | Session-cookie signing secret, at least 32 characters |
| `POSTGRES_HOST`, `POSTGRES_PORT` | PostgreSQL host and port; use `db` and `5432` with the supplied Compose setup |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | PostgreSQL credentials and database name |

Generate a random session key with `openssl rand -hex 32` and paste the output
into `SESSION_KEY`. This generates 64 hexadecimal characters; the application
uses that string directly as the signing key.

### Admin and reverse proxy

`ADMIN_EMAIL` and `ADMIN_PASSWORD` are authoritative bootstrap credentials: on
startup WebhookHub creates the single admin or updates that account to match the
configured values. Login uses a signed session cookie and bcrypt password checks;
state-changing admin requests require a CSRF token.

Production deployments behind HTTPS should set `COOKIE_SECURE=true` (default:
`false`). PostgreSQL TLS can be configured with `POSTGRES_SSLMODE` (default:
`disable`).

Failed logins are limited to 10 per 15-minute window per client IP; in-flight
password checks also occupy slots. Successful login clears recorded failures
without dropping in-flight checks. Counters are in memory, reset on restart, and
are independent for each replica.

Enable `TRUST_PROXY_HEADERS=true` only behind one trusted reverse proxy that
appends the actual client IP to `X-Forwarded-For` or overwrites the header. Direct
client access to WebhookHub must be blocked. The limiter uses the last address,
validates and normalizes it, and falls back to the connection address if it is
invalid. With a chain of proxies, clients may share a bucket. Forwarded headers
are ignored by default (`TRUST_PROXY_HEADERS=false`).

### HTTP and delivery workers

```dotenv
PORT=8080
MAX_BODY_BYTES=1048576
DELIVERY_WORKERS=4
DELIVERY_POLL_INTERVAL=1s
DELIVERY_LEASE_DURATION=30s
DELIVERY_TIMEOUT=5s
SHUTDOWN_TIMEOUT=10s
```

These are the defaults. `MAX_BODY_BYTES` limits each incoming webhook body to
1 MiB by default. `PORT` must be from 1 to 65535; body size, worker count and
intervals must be positive. `DELIVERY_LEASE_DURATION` must be at least 10 seconds.

`DELIVERY_TIMEOUT` limits a single HTTP delivery attempt. It must leave at least
5 seconds within `DELIVERY_LEASE_DURATION` for finalization (for example, timeout
5s and lease 10s). Preparation time also consumes the lease: the HTTP deadline is
capped at the remaining lease minus those 5 seconds. Finalization has its own
deadline of at most 5 seconds, never beyond the lease, including when a worker is
cancelled.

Each claim increments an internal lease version. Creating an attempt and
atomically saving its result and webhook state require the current unexpired
lease; an old worker cannot overwrite a newer owner's result. If finalization
fails or the lease expires, recovery happens through the existing reclaim
mechanism. Workers stop during graceful shutdown, bounded by `SHUTDOWN_TIMEOUT`.

## Delivery behavior

WebhookHub persists an accepted webhook before returning `202 Accepted`; workers
deliver it asynchronously. Each source has one forwarding target. If no rule
exists when a worker processes an event, it is marked `skipped` and remains
available for inspection. Add a rule and Replay it to request delivery.

Use [Pause / Resume](forwarding-ui.md#pause-and-resume) on a forwarding rule to
hold its queue while continuing to accept incoming events. Pausing prevents new
claims, including retries and recovery of expired leases, without consuming
attempts. Already claimed deliveries may finish. Paused state is stored in
PostgreSQL and survives restarts and rule edits. Resume allows queued events to
be claimed when due; it does not requeue completed or dead-lettered events.

Delivery is at-least-once. A database lease lets another worker recover a webhook
left in `processing` after a crash. A duplicate remains possible if the target
accepted a request but WebhookHub stopped before persisting the result. Receivers
must tolerate duplicates.

Retries use the rule's maximum attempt count and exponential backoff. Defaults
are 3 attempts and a 2-second initial backoff; delays are capped at 30 seconds.
Exhausted deliveries enter the dead-letter queue. Replay resets the delivery
state and queues the event again, keeping its attempt history. It uses the current
forwarding rule. See [forwarding setup](forwarding-ui.md) and
[DLQ recovery](reference.md#bulk-recovery-from-dlq).

Replay and delete requests for a webhook currently in `processing` are rejected
with HTTP `409 Conflict` so an active delivery cannot be changed underneath a
worker.

Only HTTP `2xx` responses count as successful delivery. Redirects are not followed:
the original `3xx` response, including its headers and body, is saved as a failed
attempt and uses the configured retry policy. Configure the final receiver URL
directly; payloads and outgoing signatures are never sent to a redirect destination.

Forwarding preserves the stored payload bytes and received `Content-Type`.
Other incoming headers are not copied. See [Inspect a webhook](reference.md#inspect-a-webhook)
for header handling and response capture, and [HMAC signatures](reference.md#hmac-signatures)
for the supported signing format.

## Health checks

- `GET /healthz` reports that the process is running.
- `GET /readyz` reports readiness only when PostgreSQL responds.
- The container healthcheck runs `webhookhub healthcheck`, which checks `/readyz`
  without requiring shell utilities in the image.

The supplied [Compose setup](../docker-compose.yml) waits for PostgreSQL health
before starting the application and allows 15 seconds for the container to stop.
If changing `PORT` or `SHUTDOWN_TIMEOUT`, also adjust Compose's port mapping or
`stop_grace_period` as appropriate.

## Retention cleanup

Retention cleanup is optional and disabled by default. Configure via `.env`:

```dotenv
RETENTION_ENABLED=false
RETENTION_DAYS=30
RETENTION_INTERVAL=24h
RETENTION_BATCH_SIZE=300
```

When enabled, cleanup runs once on startup and then every configured interval,
removing expired webhooks in batches by `received_at`. Active deliveries and
queued events, including those held by a pause, are retained. Cleanup locks each
batch before deleting it: a webhook successfully requeued first is retained;
Replay returns `404` if cleanup deleted the webhook first. Attempt history and
saved responses are deleted with their webhook.

HTTP database operations use the request context, so disconnecting the client
cancels outstanding work. Retention uses its worker context and stops active
queries and further batches during shutdown.

## Database and upgrades

Startup runs GORM migrations automatically and creates or replaces the PostgreSQL
function `webhookhub_payload_text(bytea)` for payload text search. The database
user must have permission to create functions in the application's schema and
own the function on upgrades.

The forwarding pause column is added automatically with a default of `false`, so
existing rules remain active. Stop all old application instances before relying
on Pause: older workers do not check the new flag and can still deliver events.

When upgrading from a version without lease ownership checks, stop all old
workers before starting the new version. Startup adds the lease-version column
automatically; mixed old/new workers are not supported during this upgrade.

The JSON API now defaults to the first 20 events. Clients that previously read
the whole journal must follow pagination links; see the
[pagination upgrade note](reference.md#paginated-json-api).

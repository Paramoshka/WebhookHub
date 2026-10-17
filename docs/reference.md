# UI and HTTP reference

[Back to README](../README.md) · [Operations](operations.md) · [Forwarding setup](forwarding-ui.md)

## Receive a webhook

Send `POST /hook/{source}` with the original payload. The source must be 1–64
ASCII letters, digits, dots, underscores or dashes, starting with a letter or
digit. Requests larger than `MAX_BODY_BYTES` return `413`; the default is 1 MiB.
Incoming HMAC verification is optional and configured per source.

A `202 Accepted` response means the event was saved, not that the target accepted
it. Persistence failures return `503`. Forwarding happens asynchronously; without
a forwarding rule, the worker marks the event `skipped`. It can be replayed after
adding a rule. See [delivery behavior](operations.md#delivery-behavior).

## HMAC signatures

Configure incoming verification and outgoing signing independently on
`/forwarding`. Both use the Stripe-style `t=<unix-seconds>, v1=<hex-signature>`
format: HMAC-SHA256 over `<timestamp>.<raw-payload>`. This is a specific format;
changing the header name does not add support for other providers' signing schemes.

Incoming verification defaults to `Stripe-Signature` with a 300-second timestamp
tolerance. A tolerance of `0` disables the timestamp check. If verification is
configured, missing, malformed or invalid signatures return `401` before the
webhook is stored. Outgoing signatures use `X-WebhookHub-Signature` with the
configured outgoing secret and a new timestamp for each delivery attempt.

## Inspect a webhook

Open **Inspect** from the logs, DLQ, or metrics to view `/webhooks/{id}`. The page
shows request headers, JSON with Formatted/Raw views, copy controls, and delivery
history. Status, attempts, and the latest saved response refresh every five
seconds without replacing the request payload. Replay queues a new delivery;
it does not mean the receiver has accepted the webhook yet.

Open an attempt ID to inspect its saved response headers and body. Each new
attempt keeps its own response, including across Replay. Bodies are limited to
1 MiB and the page indicates truncation; partial bodies are retained when reading
fails. Older attempts explicitly show that no response was saved. Attempt
responses are removed with their webhook by deletion or retention cleanup.

Forwarding and Replay preserve the received `Content-Type`, including charset
and multipart boundary parameters. If the incoming request omitted this header,
the forwarded request also omits it. Legacy records with no stored headers keep
the previous `application/json` default. Other incoming headers are not copied;
outgoing HMAC signatures are generated using the configured outgoing secret.

**Download payload** (`GET /webhooks/{id}/payload`, login required) saves the exact
stored bytes. Binary bodies show a hex preview of up to 256 bytes. Clipboard
access depends on browser permissions and a secure context (HTTPS or localhost);
if copying is unavailable, the page provides a manual-copy message. Existing
`/partials/webhook/{id}` links continue to open the full detail page.

Dashboard and DLQ lists load event metadata; full request and latest response
contents remain available through Inspect and the JSON API.

## Bulk recovery from DLQ

On `/dlq`, select individual rows or **Select all on this page**, then choose
**Requeue selected**. Selection covers at most the 20 events on the current page.
After submission, the page keeps its filters and page number and reports how many
events were queued and how many were skipped because they were deleted or had
already left the DLQ. Queuing does not mean delivery has succeeded.

`POST /api/webhooks/replay/bulk` requires login and CSRF protection. Submit a form
with repeated positive `ids` values and optional `redirect_to` pointing to `/dlq`
with its query parameters. The form is limited to 64 KiB and 20 unique IDs;
duplicates count once. Empty or invalid selections return `400`.

Eligible events are requeued in one transaction, keeping their attempt history.
Events no longer in `dead_lettered` are skipped. Database errors roll back the
transaction and return `503`; successful requests return `303` to the DLQ with
the result counts. Reload the list before retrying after an ambiguous connection
failure.

## HTTP endpoints and authentication

Admin pages and endpoints require the `session` cookie issued by login.
Ordinary unauthenticated requests redirect to `/login`; HTMX requests return
`401` with `HX-Redirect: /login`. Mutations also require the session's CSRF token
in `X-CSRF-Token` or the `csrf_token` form field. Login itself requires the token
and cookie from `GET /login`. The browser UI handles these cookies and tokens.

| Method and path | Behavior |
| --- | --- |
| `POST /hook/{source}` | Receive a webhook; no admin login, optional per-source HMAC |
| `GET /healthz`, `GET /readyz` | Public process and database health checks |
| `GET /api/webhooks` | Paginated JSON events, including payload and latest response |
| `POST /api/webhooks/replay?id={id}` | Requeue one event; optional local `redirect_to` |
| `POST /api/webhooks/delete?id={id}` | Delete an event and its attempt history; optional local `redirect_to` |
| `POST /api/webhooks/replay/bulk` | Requeue selected dead-lettered events using form data |
| `POST /forwarding/pause`, `POST /forwarding/resume` | Set a source's delivery state using a `source` form field |
| `GET /webhooks/{id}` | HTML inspection page |
| `GET /webhooks/{id}/attempts/{attemptID}` | HTML page for a saved attempt response |
| `GET /webhooks/{id}/payload` | Download the stored payload bytes |
| `GET /partials/webhooks`, `GET /partials/metrics` | HTML fragments for dashboard refresh |
| `GET /partials/webhook/{id}/delivery` | HTML fragment for delivery status and history |

Replay and delete return `404` for a missing event, `409` for an event currently
in `processing`, and `503` for database errors. Ordinary successful requests
redirect with `303`; with `HX-Request: true`, Replay returns an HTML confirmation
and Delete returns an empty `200` response. These mutation endpoints use form
and redirect responses, not JSON.

Pause and Resume require login and CSRF protection. They return `303` to
`/forwarding` on success, `400` for an invalid source or form, `404` for a missing
rule, and `503` for database errors. Repeating the same action keeps the requested
state. See [Pause and resume](forwarding-ui.md#pause-and-resume) for queue behavior.

## Paginated JSON API

`GET /api/webhooks` requires login and returns a JSON array with the existing event
fields, including payload and response bytes. It accepts the same filters and sort
options as the logs, plus `page` (default `1`) and `limit` (default `20`, range
`1–100`). Invalid pagination values and offset overflow return `400`.

When more events are available, the `Link` response header contains a relative
next-page URL with `rel="next"`, preserving filters and page size. The final page
has no next link; an empty page returns `[]`. The default sort is `id_desc`;
time-based sorts use ID as a tie-breaker.

**Upgrade note:** requests without pagination parameters now return only the first
20 events. Clients that previously read the whole journal must follow the next
links. Pages are live views: new events and deletions can shift subsequent pages.

Byte slices such as `Payload` and `Response` are base64-encoded in JSON;
`Headers` is a string containing a JSON object. The response contains event
records, not the full per-attempt response history. Downloading an individual
payload and reading paginated JSON do not provide a dedicated journal export.

## Advanced logs filters (UI and API)

`/dashboard` includes advanced filters in the UI and the same parameters are available via:

`GET /partials/webhooks`

Query params:

- `source`: exact match on webhook source.
- `status`: exact match on status (`pending`, `processing`, `retrying`, `success`, `failed`, `skipped`, `dead_lettered`).
- `q`: case-insensitive text matching across `source`, payload, headers, last error, and DLQ reason.
- `from`: inclusive lower bound for `received_at` (supports `RFC3339`, `RFC3339Nano`, `2006-01-02T15:04:05`, `2006-01-02T15:04`, `2006-01-02`).
- `to`: exclusive upper bound for `received_at` (same formats; a date-only value includes that entire calendar day by using the start of the following day in the specified timezone as the bound).
- `tz`: named timezone for values without an offset, e.g. `Europe/Moscow` (default `UTC`). Also supported by `/dashboard`, `/dlq`, and `/api/webhooks`.
- `sort`: one of `id_desc` (default), `received_desc`, `received_asc`, `id_asc`.
- `page`: page number (default `1`), 10 items per page in the logs.
  The JSON API defaults to 20 items and accepts `limit`; DLQ pages contain 20 items.

### Timezones and dates

The **Timezone** selector in the header defaults to **Browser timezone** and offers
UTC and named timezones. Search by city or timezone name, for example `Moscow`,
`new york`, or `Asia/Tokyo`; matching ignores case. Arrow keys and Enter select
a result, and Escape closes the list without changing the current timezone.
Browsers without the Popover API use a native select instead of the searchable
popup. The choice is saved in this browser's local storage and
applies to timestamps throughout the UI and the **From / To** controls. Timestamps
show seconds and their UTC offset; hovering also reveals the timezone and original
UTC value. Without JavaScript, rendered timestamps and date inputs remain in UTC.
The full timezone list requires a browser with `Intl.supportedValuesOf`; older
browsers offer UTC, the browser timezone, and any saved timezone.

Changing timezone preserves the applied interval. Unsubmitted date edits retain
their entered clock values in the newly selected timezone. Applying filters writes
exact RFC3339 UTC bounds into navigation links, so shared links, pagination, and
automatic refresh keep the same interval regardless of the viewer's timezone.
Reapplying unchanged date fields preserves the original fractional seconds.

The calendar buttons beside **From / To** open a white, rounded date-and-time
picker. Choose a day and time, then **Done**; **Clear** removes the bound. Escape
or clicking outside dismisses unconfirmed changes. Arrow keys navigate days,
Page Up/Down change months, and Shift + Page Up/Down change years. Browsers
without the Popover API keep their native calendar; manual date input works in
either case.

Existing URLs without `tz` or an explicit offset still mean UTC, independent of
the server timezone. RFC3339 values with an explicit offset always keep their
original instant. Invalid dates, unknown timezones, and clock times skipped or
repeated during a timezone transition return `400`. The filter form displays the
error while keeping the previous results. Choose a time outside the transition,
or use an RFC3339 URL with an explicit offset to select a repeated time precisely.

### Refresh and navigation

Automatic refresh preserves the current page and applied filters. Apply and Reset
start at page 1; Reset clears the filters.

Applying filters and changing pages updates the `/dashboard` URL. Reloading,
opening a shared URL, and browser Back/Forward restore the selected filters and
page. Inspect links from logs and DLQ retain a return link to that result set,
including after Replay. Background refresh does not add browser history entries.

**Pause updates** pauses background refresh for logs and metrics. Filters,
pagination, and actions still work while paused; **Resume updates** fetches fresh
data immediately. Each section shows its last successful refresh time and reports
refresh failures while retaining the displayed data. Retry/Delete report progress
and errors and prevent repeated clicks while an action is running. Action results
refresh the view once even while background updates are paused.

### Text search

Payload search is case-insensitive text matching over UTF-8 content, including
existing events. Binary payloads and bodies containing zero bytes are excluded
from payload text matching, but remain searchable by their other fields and are
stored and forwarded unchanged. See
[database permissions](operations.md#database-and-upgrades) for the payload-search
function requirements.

Example using a previously authenticated session cookie saved in `cookies.txt`:

```bash
curl --cookie cookies.txt "http://localhost:8080/partials/webhooks?source=stripe&status=failed&q=payment&from=2026-07-01&to=2026-07-11T23:59&sort=received_desc&page=2"
```

# 🌐 Expose WebhookHub for Local Development

WebhookHub does not include a built-in tunnel integration. For local development, use an external tunnel tool such as ngrok, localtunnel, or cloudflared.

## Option 1: ngrok

### Step 1: Start WebhookHub locally

```bash
go run ./cmd/webhookhub
```

### Step 2: Start ngrok

```bash
ngrok http 8080
```

You'll get a public URL like:

```text
https://abc123.ngrok.io
```

### Step 3: Use the public URL in external services

Example for Stripe:

```text
https://abc123.ngrok.io/hook/stripe
```

## Notes

- This is a manual local-dev workflow, not an application feature
- If you run WebhookHub in Docker, make sure port `8080` is exposed to the host first
- For repeated failure testing, point a source to a target that returns `4xx` or `5xx` and inspect `/dlq`

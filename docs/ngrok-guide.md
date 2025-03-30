# 🌐 Expose WebhookHub Using ngrok

You can expose your local WebhookHub to receive real webhook events from the internet.

## Step 1: Start WebhookHub locally

```bash
go run ./cmd/webhookhub
```

## Step 2: Start ngrok

```bash
ngrok http 8080
```

You'll get a public URL like:

```
https://abc123.ngrok.io
```

## Step 3: Use the public URL in external services

Example for Stripe:

```
https://abc123.ngrok.io/hook/stripe
```

## Tip

If you're on Linux/Mac, you can install ngrok via:

```bash
brew install ngrok
```
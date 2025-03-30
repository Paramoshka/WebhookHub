# 🔁 Configure Forwarding in Web UI (Planned)

🚧 In future versions, WebhookHub will support configuring forwarding rules via Web UI.

Planned features:
- Add/edit forwarding URLs per source
- Enable/disable targets
- Set delivery filters
- View delivery attempts per target

## For now

Forwarding is configured statically in:

```
internal/config/config.go
```

Example:
```go
func GetForwardTarget(source string) string {
    switch source {
    case "stripe":
        return "http://localhost:9000/stripe"
    default:
        return ""
    }
}
```

UI config panel coming soon!
# Example Plugin

A complete example plugin demonstrating how to build Aroute CMS plugins using the SDK.

## What This Plugin Demonstrates

1. **Service Access** — Retrieve the DatabaseService via `sdk.GetDB()` helper
2. **Event Subscription** — Listen to `content.*.created` events via `sdk.SubscribeEvent()`
3. **Route Registration** — Add custom HTTP endpoints via `sdk.GetRouter()`

## Building

```bash
# From the Aroute project root:
go build ./sdk/go/example/
```

## Plugin Structure

```
example/
├── plugin.go    # Main plugin implementation
└── README.md    # This file
```

## API Endpoints

When running, the plugin registers these endpoints:

- `GET /api/plugins/example/hello` — Returns a greeting message
- `GET /api/plugins/example/info` — Returns plugin and SDK metadata

## Key Patterns

### Embedding BasePlugin

```go
type MyPlugin struct {
    *sdk.BasePlugin
    // your custom fields...
}
```

### Accessing Services

```go
func (p *MyPlugin) Init(ctx core.CoreContext) error {
    p.BasePlugin.Init(ctx) // store context

    db, err := sdk.GetDB(ctx.Services())
    if err != nil {
        return err
    }
    // use db...
    return nil
}
```

### Subscribing to Events

```go
handlerID := sdk.SubscribeEvent(ctx, "content.*.created",
    func(ctx context.Context, event events.Event) {
        log.Printf("Content created: %v", event.Data["id"])
    },
)
```

### Registering Routes

```go
router, _ := sdk.GetRouter(ctx.Services())
router.HandleFunc("/api/plugins/my-plugin/hello", myHandler)
```

## Using as a Template

Copy this directory to start a new plugin:

```bash
cp -r sdk/go/example/ plugins/my-plugin/
# Edit plugin name, version, and add your logic
```

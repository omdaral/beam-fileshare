# Hotspot Integration with the Server (Go)

> The network module is now part of the binary: `goserver/hotspot*.go` + `goserver/netclients.go` + `goserver/runcmd.go` (stdlib only).

## 1. Example server call

```go
// Start networking (default SSID Beam, 8+ character password)
ok, msg, info := hotspotStart("Beam", "password123", 2004, false)
fmt.Println(msg)
if ok {
    fmt.Println("URL:", info["url"]) // example: http://192.168.137.1:2004
} else {
    // Hotspot impossible? Use LAN fallback mode
    fb := lanFallbackInfo(2004)
    fmt.Println(fb["message_ar"], fb["url"])
}

// Status and stop
fmt.Println(hotspotStatus()) // {running, ssid, ip, port, clients}
ok, msg = hotspotStop()
```

## 2. Command-line flags (in `goserver/main.go`)

```
--hotspot                start hotspot before server
--ssid Beam     network name
--password password123   hotspot password (8+ chars)
--open                   open network with no password (Linux only)
--lan-mode               skip hotspot and work on the current network
--port 2004              fixed port (default 2004)
--lang ar                default language for new visitors (ar/en)
--no-browser             do not open the browser automatically at startup
--idle-timeout 5h        auto shutdown after idle (0 to disable)
```

- `--hotspot --ssid Beam --password password123` → calls `hotspotStart(...)` then starts the server.
- `--lan-mode` (or `start` failure) → calls `lanFallbackInfo(port)` and prints `url` for clients.
- On stop (Ctrl+C) → calls `hotspotStop()`.
- From the browser (host machine): `POST /api/net/start` with the same options — and if privileges are missing
  the server tries `pkexec` automatically (system popup on your machine only), otherwise falls back to LAN.

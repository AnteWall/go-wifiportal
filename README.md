# go-wifiportal

A simple Go library for creating WiFi access points with captive portals for network configuration.

## Features

- Create WiFi access points (hotspots)
- Web-based portal for WiFi network setup
- Interface management for wireless devices
- Built-in DHCP and DNS configuration
- Iptables/UFW firewall integration

## Installation

```bash
go get github.com/AnteWall/go-wifiportal
```

## Quick Example

```go
package main

import (
    "context"
    "log/slog"
    
    "github.com/AnteWall/go-wifiportal/pkg/portal"
)

func main() {
    // Create a simple WiFi setup portal
    config := portal.Config{
        Port:      "8080",
        Interface: "wlan0",
        SSID:      "Setup-Portal",
        Gateway:   "192.168.4.1",
    }
    
    server := portal.NewServer(config)
    
    slog.Info("Starting WiFi portal on :8080")
    if err := server.Start(context.Background()); err != nil {
        slog.Error("Failed to start server", "error", err)
    }
}
```

## Usage

The portal will create a WiFi access point that users can connect to. When they visit any website, they'll be redirected to a setup page where they can configure the device to connect to their preferred WiFi network.

## Requirements

- Linux system with wireless capabilities
- Root privileges (for network interface management)
- `hostapd`, `dnsmasq` (for access point functionality)

## Examples

See the `examples/` directory for more complete implementations including full access point setup with NetworkManager integration.

## License

MIT License - see [LICENSE](LICENSE) file for details.

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AnteWall/go-wifiportal/pkg/network"
	"github.com/pkg/errors"
)

func main() {
	// Set up logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	// Find the best wireless interface for creating an access point
	iFace, err := network.NewInterfaceManager().GetBestAPInterface()
	if err != nil {
		if iFace != nil {
			slog.Info("interface found", slog.String("name", iFace.Name))
		}
		if errors.Is(err, network.ErrAllAccessPointsInUse) {
			slog.Warn("All access points are in use, using the first available interface", slog.String("interface", iFace.Name))
		} else {
			slog.Error("failed to find suitable interface", slog.String("error", err.Error()))
			return
		}
	}
	slog.Info("Using interface", slog.String("name", iFace.Name))

	// Create NetworkManager-based hotspot service
	h := network.NewHostAPDService()
	ctx := context.Background()
	
	// Configure the access point
	config := network.APConfig{
		Name:        "go-wifiportal",
		Interface:   iFace.Name,
		SSID:        "GoWiFiPortal",
		Password:    "12345678",
		Channel:     6,
		CountryCode: "SE",
		Security:    "WPA2",
		Gateway:     "192.168.4.1",
		DHCPRange:   "192.168.4.2,192.168.4.50",
	}

	// Start the hotspot
	slog.Info("Starting WiFi hotspot with NetworkManager...")
	if err := h.Start(ctx, config); err != nil {
		slog.Error("failed to start hotspot", slog.String("error", err.Error()))
		return
	}

	slog.Info("WiFi hotspot started successfully!")
	slog.Info("SSID: " + config.SSID)
	slog.Info("Gateway: " + config.Gateway)
	slog.Info("DHCP Range: " + config.DHCPRange)

	// Set up graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Wait for signal or timeout
	select {
	case <-c:
		slog.Info("Received interrupt signal, shutting down...")
	case <-time.After(60 * time.Second):
		slog.Info("Demo timeout reached, shutting down...")
	}

	// Stop the hotspot
	slog.Info("Stopping WiFi hotspot...")
	if err := h.Stop(ctx); err != nil {
		slog.Error("failed to stop hotspot", slog.String("error", err.Error()))
	} else {
		slog.Info("WiFi hotspot stopped successfully")
	}
}

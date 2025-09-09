package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AnteWall/go-wifiportal/pkg/network"
	"github.com/AnteWall/go-wifiportal/pkg/portal"
	"github.com/pkg/errors"
)

func main() {
	// Set up logging
	slog.SetLogLoggerLevel(slog.LevelDebug)

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
	h := network.NewAPService()
	ctx := context.Background()

	// Configure the access point
	apConfig := network.APConfig{
		Name:        "go-wifiportal",
		Interface:   iFace.Name,
		SSID:        "GoWiFiPortal",
		Password:    "12345678",
		CountryCode: "SE",
		Security:    "wpa2",
		Gateway:     "192.168.4.1",
		DHCPRange:   "192.168.4.2,192.168.4.50",
		PortalPort:  "8080", // Portal runs on port 8080, traffic redirected from 80
	}

	// Configure the captive portal server
	portalConfig := portal.Config{
		Port:        apConfig.PortalPort,      // Use the same port configured in AP
		Gateway:     apConfig.Gateway,         // Same as AP gateway
		SSID:        apConfig.SSID,            // Same as AP SSID
		RedirectURL: "https://www.google.com", // Where to redirect after login
	}

	// Create the portal server
	portalServer := portal.NewServer(portalConfig)

	// Add custom routes if needed
	portalServer.AddRoute("/api/custom", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Custom endpoint", "status": "ok"}`))
	})

	// Start the WiFi hotspot
	slog.Info("Starting WiFi hotspot...")
	if err := h.Start(ctx, apConfig); err != nil {
		slog.Error("failed to start hotspot", slog.String("error", err.Error()))
		return
	}
	slog.Info("WiFi hotspot started successfully!")

	// Start the captive portal server
	slog.Info("Starting captive portal server...")
	if err := portalServer.Start(ctx); err != nil {
		slog.Error("failed to start portal server", slog.String("error", err.Error()))
		// Stop the hotspot if portal fails
		h.Stop(ctx)
		return
	}
	slog.Info("Captive portal server started successfully!")

	// Log the complete setup
	slog.Info("=== WiFi Setup Portal Active ===")
	slog.Info("SSID: " + apConfig.SSID)
	slog.Info("Password: " + apConfig.Password)
	slog.Info("Security: WPA2 with AES encryption")
	slog.Info("Gateway: " + apConfig.Gateway)
	slog.Info("DHCP Range: " + apConfig.DHCPRange)
	slog.Info("Portal URL: http://" + apConfig.Gateway + " (redirected to port " + apConfig.PortalPort + ")")
	slog.Info("======================================")
	slog.Info("Connect to WiFi and navigate to any website to configure device WiFi!")
	slog.Info("All HTTP traffic is redirected to the configuration portal")

	// Set up graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Wait for signal
	<-c
	slog.Info("Received interrupt signal, shutting down...")

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop the portal server
	slog.Info("Stopping captive portal server...")
	if err := portalServer.Stop(shutdownCtx); err != nil {
		slog.Error("failed to stop portal server", slog.String("error", err.Error()))
	} else {
		slog.Info("Captive portal server stopped successfully")
	}

	// Stop the hotspot
	slog.Info("Stopping WiFi hotspot...")
	if err := h.Stop(ctx); err != nil {
		slog.Error("failed to stop hotspot", slog.String("error", err.Error()))
	} else {
		slog.Info("WiFi hotspot stopped successfully")
	}

	slog.Info("Shutdown complete")
}

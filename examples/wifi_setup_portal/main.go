package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/AnteWall/go-wifiportal/pkg/portal"
)

func main() {
	// Configure structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// Portal configuration
	config := portal.Config{
		Port:      "8080",
		Interface: "wlan0", // You can change this to your WiFi interface
		SSID:      "WiFi-Setup-Portal",
		Gateway:   "192.168.4.1",
	}

	// Create and start the WiFi setup portal server
	server := portal.NewServer(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the server
	if err := server.Start(ctx); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	logger.Info("WiFi Setup Portal started",
		slog.String("address", "http://localhost:"+config.Port),
		slog.String("interface", config.Interface))

	logger.Info("Available endpoints:")
	logger.Info("  - Main setup page: http://localhost:" + config.Port + "/setup")
	logger.Info("  - Connection status: http://localhost:" + config.Port + "/status")
	logger.Info("  - API networks: http://localhost:" + config.Port + "/api/networks")
	logger.Info("  - API interfaces: http://localhost:" + config.Port + "/api/interfaces")

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down WiFi Setup Portal...")

	// Stop the server
	if err := server.Stop(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	logger.Info("WiFi Setup Portal stopped")
}

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/AnteWall/go-wifiportal/pkg/network"
)

func main() {

	iFace, err := network.NewInterfaceManager().GetBestAPInterface()
	if err != nil {
		slog.Error(err.Error())
		return
	}
	slog.Info("Using interface: " + iFace.Name)

	h := network.NewHostAPDService()
	ctx := context.Background()
	if err := h.Start(ctx, network.APConfig{
		Name:        "testing",
		Interface:   iFace.Name,
		SSID:        "TestSSID",
		Password:    "12345678",
		Channel:     6,
		CountryCode: "SE",
		Security:    "WPA2",
		Gateway:     "192.168.4.1",
		DHCPRange:   "192.168.4.2,192.168.4.50",
	}); err != nil {
		slog.Error(err.Error())
		return
	}

	time.Sleep(10 * time.Second)

	if err := h.Stop(ctx); err != nil {
		slog.Error(err.Error())
	}

}

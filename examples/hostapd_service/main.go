package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/AnteWall/go-wifiportal/pkg/network"
	"github.com/pkg/errors"
)

func main() {

	iFace, err := network.NewInterfaceManager().GetBestAPInterface()
	if err != nil {
		slog.Info(iFace.Name)
		if errors.Is(err, network.ErrAllAccessPointsInUse) {
			slog.Warn("All access points are in use, using the first available interface %s", iFace.Name)
		} else {
			slog.Error(err.Error())
			return
		}
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
	defer func(h network.HostAPDService, ctx context.Context) {
		err := h.Stop(ctx)
		if err != nil {
			slog.Error(err.Error())
		}
	}(h, ctx)
	time.Sleep(10 * time.Second)
}

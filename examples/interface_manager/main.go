package main

import (
	"log/slog"

	"github.com/AnteWall/go-wifiportal/pkg/network"
)

func main() {
	im := network.NewInterfaceManager()

	interfaces, err := im.ListWirelessInterfaces()
	if err != nil {
		slog.With("slog", "error").Error("Failed to list wireless interfaces", "error", err)
		return
	}

	for _, iface := range interfaces {
		slog.With(slog.Any("interface", iface)).Info("Found wireless interface")
	}

	bestInterface, err := im.GetBestAPInterface()
	if err != nil {
		slog.With("slog", "error").Error("Failed to get best AP interface", "error", err)
	}
	slog.With(slog.Any("best_interface", bestInterface)).Info("Best AP interface selected")

}

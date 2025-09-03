package network

import (
	"log/slog"
	"net"
	"os/exec"

	"github.com/pkg/errors"
)

type WirelessInterface struct {
	Name       string `json:"name"`
	SupportAP  bool   `json:"support_ap"`
	InUse      bool   `json:"in_use"`
	MACAddress string `json:"mac_address"`
}

var ErrAllAccessPointsInUse = errors.New("all wireless access points are currently in use")
var ErrNoAccessPointFound = errors.New("no wireless access point found")

type InterfaceManager interface {
	ListWirelessInterfaces() ([]WirelessInterface, error)
	GetBestAPInterface() (*WirelessInterface, error)
}

type interfaceManager struct {
	logger *slog.Logger
}

// NewInterfaceManager creates a new instance of InterfaceManager
func NewInterfaceManager() InterfaceManager {
	return &interfaceManager{
		logger: slog.Default().With("component", "interface_manager"),
	}
}

func (im *interfaceManager) ListWirelessInterfaces() ([]WirelessInterface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list network interfaces")
	}
	var wirelessInterfaces []WirelessInterface
	for _, i := range interfaces {
		if im.isWireless(i.Name) {
			wirelessInterfaces = append(wirelessInterfaces, WirelessInterface{
				Name:       i.Name,
				MACAddress: i.HardwareAddr.String(),
				InUse:      i.Flags&net.FlagUp != 0,
				SupportAP:  im.supportsAPMode(i.Name),
			})
		}
	}
	return wirelessInterfaces, nil
}

func (im *interfaceManager) GetBestAPInterface() (*WirelessInterface, error) {
	interfaces, err := im.ListWirelessInterfaces()
	if err != nil {
		return nil, err
	}
	// Check after unused interfaces that support AP mode
	for _, i := range interfaces {
		if i.SupportAP && !i.InUse {
			return &i, nil
		}
	}
	// return any interface that supports AP modem but return an error
	for _, i := range interfaces {
		if i.SupportAP {
			return &i, ErrAllAccessPointsInUse
		}
	}
	return nil, ErrNoAccessPointFound
}

func (im *interfaceManager) isWireless(i string) bool {
	cmd := exec.Command("test", "-d", "/sys/class/net/"+i+"/wireless")
	err := cmd.Run()
	return err == nil
}

func (im *interfaceManager) supportsAPMode(i string) bool {
	cmd := exec.Command("iw", "list")
	output, err := cmd.Output()
	if err != nil {
		im.logger.Error("failed to check AP mode support", slog.String("interface", i), slog.String("error", err.Error()))
		return false
	}
	return containsAPMode(string(output))
}

func containsAPMode(iwOutput string) bool {
	// Simplified check for AP mode support
	return contains(iwOutput, "AP")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[0:len(substr)] == substr || contains(s[1:], substr)))
}

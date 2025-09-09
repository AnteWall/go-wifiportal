package network

import (
	"log/slog"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type WirelessInterface struct {
	Name       string `json:"name"`
	SupportAP  bool   `json:"support_ap"`
	InUse      bool   `json:"in_use"`
	MACAddress string `json:"mac_address"`
}

type WirelessNetwork struct {
	SSID        string `json:"ssid"`
	DisplayName string `json:"display_name"` // Human-readable name (same as SSID for now)
	BSSID       string `json:"bssid"`
	Signal      int    `json:"signal"`
	Security    string `json:"security"`
	Frequency   string `json:"frequency"`
	Channel     string `json:"channel"`
}

var ErrAllAccessPointsInUse = errors.New("all wireless access points are currently in use")
var ErrNoAccessPointFound = errors.New("no wireless access point found")
var ErrNetworkNotFound = errors.New("specified network not found")
var ErrConnectionFailed = errors.New("failed to connect to network")

type InterfaceManager interface {
	ListWirelessInterfaces() ([]WirelessInterface, error)
	GetBestAPInterface() (*WirelessInterface, error)
	ListAvailableNetworks(interfaceName string) ([]WirelessNetwork, error)
	ConnectToNetwork(interfaceName, ssid, password string) error
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

func (im *interfaceManager) ListAvailableNetworks(interfaceName string) ([]WirelessNetwork, error) {
	im.logger.Info("scanning for networks", slog.String("interface", interfaceName))
	
	// Check if nmcli is available
	if _, err := exec.LookPath("nmcli"); err != nil {
		return nil, errors.New("nmcli (NetworkManager) is not installed or not available in PATH")
	}
	
	// First try to rescan/refresh
	rescanCmd := exec.Command("nmcli", "device", "wifi", "rescan")
	if interfaceName != "" {
		rescanCmd.Args = append(rescanCmd.Args, "ifname", interfaceName)
	}
	if err := rescanCmd.Run(); err != nil {
		im.logger.Warn("failed to rescan networks", slog.String("error", err.Error()))
	}
	
	// Use nmcli to list available networks
	cmd := exec.Command("nmcli", "-t", "-f", "SSID,BSSID,MODE,CHAN,FREQ,RATE,SIGNAL,BARS,SECURITY", "device", "wifi", "list")
	if interfaceName != "" {
		cmd.Args = append(cmd.Args, "ifname", interfaceName)
	}
	
	output, err := cmd.Output()
	if err != nil {
		// If interface-specific command fails, try without interface specification
		if interfaceName != "" {
			im.logger.Warn("failed to scan with specific interface, trying all interfaces", 
				slog.String("interface", interfaceName),
				slog.String("error", err.Error()))
			cmd = exec.Command("nmcli", "-t", "-f", "SSID,BSSID,MODE,CHAN,FREQ,RATE,SIGNAL,BARS,SECURITY", "device", "wifi", "list")
			output, err = cmd.Output()
		}
		if err != nil {
			return nil, errors.Wrapf(err, "failed to scan for networks (interface: %s)", interfaceName)
		}
	}
	
	im.logger.Debug("nmcli output", slog.String("output", string(output)))
	return im.parseNetworkList(string(output))
}

func (im *interfaceManager) ConnectToNetwork(interfaceName, ssid, password string) error {
	im.logger.Info("attempting to connect to network", 
		slog.String("interface", interfaceName), 
		slog.String("ssid", ssid))

	// First, check if there's already a connection to this SSID
	if err := im.disconnectExistingConnection(ssid); err != nil {
		im.logger.Warn("failed to disconnect existing connection", slog.String("error", err.Error()))
	}

	// Connect to the network using nmcli
	var cmd *exec.Cmd
	if password == "" {
		// Open network (no password)
		cmd = exec.Command("nmcli", "device", "wifi", "connect", ssid, "ifname", interfaceName)
	} else {
		// Secured network (with password)
		cmd = exec.Command("nmcli", "device", "wifi", "connect", ssid, "password", password, "ifname", interfaceName)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to connect to network %s on interface %s: %s", ssid, interfaceName, string(output))
	}

	im.logger.Info("successfully connected to network", 
		slog.String("interface", interfaceName), 
		slog.String("ssid", ssid))

	return nil
}

func (im *interfaceManager) disconnectExistingConnection(ssid string) error {
	// Get list of active connections
	cmd := exec.Command("nmcli", "connection", "show", "--active")
	output, err := cmd.Output()
	if err != nil {
		return errors.Wrap(err, "failed to list active connections")
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, ssid) {
			// Extract connection name (first field)
			fields := strings.Fields(line)
			if len(fields) > 0 {
				connectionName := fields[0]
				// Disconnect the existing connection
				disconnectCmd := exec.Command("nmcli", "connection", "down", connectionName)
				if err := disconnectCmd.Run(); err != nil {
					return errors.Wrapf(err, "failed to disconnect existing connection %s", connectionName)
				}
				im.logger.Debug("disconnected existing connection", slog.String("connection", connectionName))
			}
		}
	}

	return nil
}

func (im *interfaceManager) parseNetworkList(output string) ([]WirelessNetwork, error) {
	var networks []WirelessNetwork
	lines := strings.Split(output, "\n")
	
	// Parse nmcli tabular output format (-t flag)
	// Format: SSID:BSSID:MODE:CHAN:FREQ:RATE:SIGNAL:BARS:SECURITY
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// Split by colon (tabular format)
		fields := strings.Split(line, ":")
		if len(fields) < 9 {
			// Try space-separated format as fallback
			fields = strings.Fields(line)
			if len(fields) < 7 {
				continue
			}
		}
		
		// Skip hidden networks (empty SSID)
		ssid := fields[0]
		if ssid == "" || ssid == "--" {
			continue
		}
		
		// Extract network information
		network := WirelessNetwork{
			SSID:        ssid,
			DisplayName: ssid, // Use SSID as display name
			BSSID:       fields[1],
		}
		
		// Parse channel (field 3)
		if len(fields) > 3 {
			network.Channel = fields[3]
		}
		
		// Parse frequency (field 4)
		if len(fields) > 4 {
			network.Frequency = fields[4]
		}
		
		// Parse signal strength (field 6)
		if len(fields) > 6 {
			signalStr := fields[6]
			// Remove dBm suffix and convert
			signalStr = strings.TrimSpace(strings.TrimSuffix(signalStr, "dBm"))
			if signal, err := strconv.Atoi(signalStr); err == nil {
				// Convert dBm to percentage (rough approximation)
				// -30dBm = 100%, -67dBm = 50%, -90dBm = 0%
				if signal >= -30 {
					network.Signal = 100
				} else if signal <= -90 {
					network.Signal = 0
				} else {
					network.Signal = int(((float64(signal) + 90) / 60) * 100)
				}
			}
		}
		
		// Parse security (field 8)
		if len(fields) > 8 {
			security := fields[8]
			if security == "" || security == "--" {
				network.Security = "none"
			} else {
				network.Security = security
			}
		} else {
			network.Security = "unknown"
		}
		
		networks = append(networks, network)
	}
	
	im.logger.Debug("parsed networks", slog.Int("count", len(networks)))
	return networks, nil
}

func (im *interfaceManager) isWireless(i string) bool {
	cmd := exec.Command("test", "-d", "/sys/class/net/"+i+"/wireless")
	err := cmd.Run()
	return err == nil
}

func (im *interfaceManager) supportsAPMode(i string) bool {
	// Check if interface supports AP mode using nmcli
	cmd := exec.Command("nmcli", "device", "wifi", "list", "ifname", i)
	if err := cmd.Run(); err != nil {
		im.logger.Debug("interface does not support wifi", slog.String("interface", i))
		return false
	}

	// If nmcli can list wifi for this interface, it likely supports AP mode
	// NetworkManager generally supports AP mode on most modern wifi interfaces
	return true
}

func containsAPMode(iwOutput string) bool {
	// Simplified check for AP mode support
	return contains(iwOutput, "AP")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[0:len(substr)] == substr || contains(s[1:], substr)))
}

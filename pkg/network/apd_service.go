package network

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/pkg/errors"
)

var (
	ErrInvalidAPConfig       = errors.New("invalid wireless wireless access point")
	ErrServiceAlreadyRunning = errors.New("hotspot service is already running")
)

//go:embed templates/*.tmpl
var templateFiles embed.FS

// APConfig represents the configuration for a wireless access point
type APConfig struct {
	Name        string
	Interface   string `yaml:"interface" json:"interface"`
	SSID        string `yaml:"ssid" json:"ssid"`
	Password    string `yaml:"password" json:"password"`
	Channel     int    `yaml:"channel" json:"channel"`
	CountryCode string `yaml:"country_code" json:"countryCode"`
	Security    string `yaml:"security" json:"security"` // "open", "wpa2" (uses AES/CCMP encryption)
	Gateway     string `yaml:"gateway" json:"gateway"`
	DHCPRange   string `yaml:"dhcp_range" json:"dhcpRange"`
	PortalPort  string `yaml:"portal_port" json:"portalPort"` // Port for captive portal server (default 8080)
}

func (c APConfig) Validate() error {
	if len(c.Name) == 0 {
		return errors.Wrap(ErrInvalidAPConfig, "name is required")
	}
	if len(c.Interface) == 0 {
		return errors.Wrap(ErrInvalidAPConfig, "interface is required")
	}
	if len(c.SSID) == 0 {
		return errors.Wrap(ErrInvalidAPConfig, "ssid is required")
	}
	if len(c.CountryCode) == 0 {
		return errors.Wrap(ErrInvalidAPConfig, "country code is required")
	}
	if len(c.Gateway) == 0 {
		return errors.Wrap(ErrInvalidAPConfig, "gateway is required")
	}
	if len(c.DHCPRange) == 0 {
		return errors.Wrap(ErrInvalidAPConfig, "DHCPRange is required")
	}
	// Password is only required for secured networks
	if c.Security != "open" && len(c.Password) == 0 {
		return errors.Wrap(ErrInvalidAPConfig, "password is required for secured networks")
	}
	// For WPA2, password must be at least 8 characters
	if c.Security == "wpa2" && len(c.Password) < 8 {
		return errors.Wrap(ErrInvalidAPConfig, "password must be at least 8 characters for WPA2")
	}
	return nil
}

type APService interface {
	Start(ctx context.Context, config APConfig) error
	Stop(ctx context.Context) error
	IsRunning() bool
}

type hostAPDService struct {
	config            APConfig
	dnsmasqConfigPath string
	dnsmasqCmd        *exec.Cmd
	running           bool
	logger            *slog.Logger
}

func NewAPService() APService {
	return &hostAPDService{
		logger:  slog.Default().WithGroup("ap_service"),
		running: false,
	}
}

func (h *hostAPDService) Start(ctx context.Context, config APConfig) error {
	if h.running {
		return ErrServiceAlreadyRunning
	}
	if err := config.Validate(); err != nil {
		return errors.Wrap(err, "invalid access point configuration")
	}
	h.config = config

	if err := h.prepareInterface(); err != nil {
		return errors.Wrap(err, "failed to prepare interface")
	}
	if err := h.createHotspot(); err != nil {
		return errors.Wrap(err, "failed to create NetworkManager hotspot")
	}
	if err := h.configureNetwork(); err != nil {
		return errors.Wrap(err, "failed to configure network")
	}
	// Start dnsmasq for DHCP and DNS with captive portal redirects
	if err := h.startDNSMasq(); err != nil {
		return errors.Wrap(err, "failed to start dnsmasq")
	}

	h.running = true
	return nil
}

func (h *hostAPDService) Stop(ctx context.Context) error {
	if !h.running {
		return nil
	}

	h.stopDNSMasq()
	h.stopHotspot()
	h.cleanupNetworkRules()

	h.running = false
	return nil
}

func (h *hostAPDService) IsRunning() bool {
	return h.running
}

func (h *hostAPDService) prepareInterface() error {
	h.logger.Debug("preparing interface for NetworkManager hotspot")

	// Stop any existing dnsmasq service
	h.logger.Debug("stopping system dnsmasq service")
	if err := exec.Command("systemctl", "stop", "dnsmasq").Run(); err != nil {
		h.logger.Warn("failed to stop system dnsmasq service", slog.String("error", err.Error()))
	}

	// Ensure the interface is managed by NetworkManager
	h.logger.Debug("ensuring interface is managed by NetworkManager")
	if err := exec.Command("nmcli", "device", "set", h.config.Interface, "managed", "yes").Run(); err != nil {
		return errors.Wrap(err, "failed to set interface to managed mode")
	}

	// Disconnect any existing connections on the interface
	h.logger.Debug("disconnecting existing connections")
	if o, err := exec.Command("nmcli", "device", "disconnect", h.config.Interface).CombinedOutput(); err != nil {
		if strings.Contains(string(o), "This device is not active") {
			h.logger.Debug("no active connection to disconnect")
		} else {
			h.logger.Warn("failed to disconnect interface", slog.String("error", err.Error()), slog.String("output", string(o)))
		}
	}

	return h.verifyInterfaceStatus(h.config.Interface)
}

// createHotspot creates a WiFi hotspot using NetworkManager with captive portal
func (h *hostAPDService) createHotspot() error {
	h.logger.Info("creating NetworkManager hotspot with captive portal",
		slog.String("interface", h.config.Interface),
		slog.String("ssid", h.config.SSID),
		slog.String("hotspot_name", h.config.Name))

	// Create hotspot connection using nmcli with manual IP configuration
	args := []string{
		"connection", "add",
		"type", "wifi",
		"ifname", h.config.Interface,
		"con-name", h.config.Name,
		"autoconnect", "yes",
		"wifi.mode", "ap",
		"wifi.ssid", h.config.SSID,
		// Let NetworkManager auto-select the best channel
		// "wifi.band", "bg", // Optional: specify band if needed
		// "wifi.channel", fmt.Sprintf("%d", h.config.Channel), // Auto-selected
		// Use manual method with explicit IP configuration
		"ipv4.method", "manual",
		"ipv4.addresses", fmt.Sprintf("%s/24", h.config.Gateway),
	}

	// Add security settings based on configuration
	if h.config.Security == "open" {
		// No security settings needed for open network
		h.logger.Debug("creating open network (no security)")
	} else if h.config.Security == "wpa2" && h.config.Password != "" {
		// WPA2 with AES (CCMP) encryption
		args = append(args,
			"wifi-sec.key-mgmt", "wpa-psk",
			"wifi-sec.proto", "rsn", // WPA2 only
			"wifi-sec.pairwise", "ccmp", // AES encryption
			"wifi-sec.group", "ccmp", // AES for group cipher
			"wifi-sec.psk", h.config.Password,
		)
		h.logger.Debug("creating WPA2-PSK network with AES encryption")
	} else if h.config.Password != "" {
		// Default to WPA2 with AES if security type not specified but password provided
		args = append(args,
			"wifi-sec.key-mgmt", "wpa-psk",
			"wifi-sec.proto", "rsn", // WPA2 only
			"wifi-sec.pairwise", "ccmp", // AES encryption
			"wifi-sec.group", "ccmp", // AES for group cipher
			"wifi-sec.psk", h.config.Password,
		)
		h.logger.Debug("creating WPA2-PSK network with AES encryption (default)")
	}

	cmd := exec.Command("nmcli", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	}

	// Wait a moment for the connection to be created
	time.Sleep(2 * time.Second)

	// Activate the hotspot connection
	h.logger.Debug("activating hotspot connection")
	cmd = exec.Command("nmcli", "connection", "up", h.config.Name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to activate hotspot: %s, %w", string(output), err)
	}

	h.logger.Info("NetworkManager hotspot created and activated successfully")
	return nil
}

// verifyInterfaceStatus verifies the interface is properly configured
func (h *hostAPDService) verifyInterfaceStatus(iFace string) error {
	checkCmd := exec.Command("nmcli", "device", "show", iFace)
	output, err := checkCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to verify interface status: %w", err)
	}
	h.logger.Debug("interface status", slog.String("output", string(output)))
	return nil
}

func (h *hostAPDService) configureNetwork() error {
	h.logger.Debug("configuring network firewall rules")

	// Enable IP forwarding
	if err := h.enableIPForwarding(); err != nil {
		h.logger.Warn("failed to enable IP forwarding", slog.String("error", err.Error()))
	}

	// Apply UFW firewall rules
	rules := GetRequiredFirewallRules(h.config.Interface, h.config.PortalPort)
	for _, rule := range rules {
		if err := rule.Apply(h.config.Interface); err != nil {
			h.logger.Warn("failed to apply firewall rule", slog.String("error", err.Error()))
		}
	}

	// Apply IPTables rules for captive portal
	ipTablesRules := CreateIPTablesRules(h.config.Interface, h.config.PortalPort)
	for _, rule := range ipTablesRules {
		if err := rule.Apply(); err != nil {
			h.logger.Warn("failed to apply iptables rule", slog.String("error", err.Error()))
		}
	}

	return nil
}

func (h *hostAPDService) startDNSMasq() error {
	h.logger.Debug("starting DNSMasq with full DHCP and captive portal configuration")

	// Stop any existing dnsmasq service
	h.logger.Debug("stopping system dnsmasq service")
	if err := exec.Command("sudo", "systemctl", "stop", "dnsmasq").Run(); err != nil {
		h.logger.Warn("failed to stop system dnsmasq service", slog.String("error", err.Error()))
	}

	tmpl, err := template.ParseFS(templateFiles, "templates/dnsmasq.conf.tmpl")
	if err != nil {
		return errors.Wrap(err, "failed to parse dnsmasq template")
	}

	file, err := os.CreateTemp("", "dnsmasq-*.conf")
	if err != nil {
		return errors.Wrap(err, "failed to create dnsmasq config file")
	}
	defer file.Close()

	if err := tmpl.Execute(file, h.config); err != nil {
		return errors.Wrap(err, "failed to execute dnsmasq template")
	}

	h.dnsmasqConfigPath = file.Name()
	h.logger.Debug("generated dnsmasq config", slog.String("path", h.dnsmasqConfigPath))

	// Wait for interface to be fully ready
	time.Sleep(3 * time.Second)

	// Start dnsmasq with full DHCP and DNS functionality
	h.dnsmasqCmd = exec.Command("sudo", "dnsmasq", "-C", h.dnsmasqConfigPath, "--keep-in-foreground")
	if err := h.dnsmasqCmd.Start(); err != nil {
		return fmt.Errorf("failed to start dnsmasq: %w", err)
	}

	// Give dnsmasq a moment to start
	time.Sleep(2 * time.Second)

	h.logger.Info("DNSMasq started with DHCP and captive portal DNS redirects")
	return nil
}

func (h *hostAPDService) stopHotspot() {
	h.logger.Debug("stopping NetworkManager hotspot")

	// Disconnect the hotspot connection
	if err := exec.Command("nmcli", "connection", "down", h.config.Name).Run(); err != nil {
		h.logger.Error("failed to disconnect hotspot", slog.String("name", h.config.Name), slog.String("error", err.Error()))
	} else {
		h.logger.Debug("hotspot disconnected")
	}

	// Delete the hotspot connection
	if err := exec.Command("nmcli", "connection", "delete", h.config.Name).Run(); err != nil {
		h.logger.Error("failed to delete hotspot connection", slog.String("name", h.config.Name), slog.String("error", err.Error()))
	} else {
		h.logger.Debug("hotspot connection deleted")
	}
}

func (h *hostAPDService) stopDNSMasq() {
	h.logger.Debug("stopping dnsmasq service")

	// Stop the dnsmasq process if we have a reference to it
	if h.dnsmasqCmd != nil && h.dnsmasqCmd.Process != nil {
		if err := h.dnsmasqCmd.Process.Kill(); err != nil {
			h.logger.Error("failed to kill dnsmasq process", slog.String("error", err.Error()))
		} else {
			h.logger.Debug("killed dnsmasq process")
		}
		// Wait for process to finish
		h.dnsmasqCmd.Wait()
		h.dnsmasqCmd = nil
	}

	// Also kill any remaining dnsmasq processes using our config file as backup
	if h.dnsmasqConfigPath != "" {
		pattern := "dnsmasq.*" + h.dnsmasqConfigPath
		if err := exec.Command("pkill", "-f", pattern).Run(); err != nil {
			h.logger.Debug("no additional dnsmasq processes found", slog.String("pattern", pattern))
		}

		// Cleanup config file
		if err := os.Remove(h.dnsmasqConfigPath); err != nil {
			h.logger.Error("failed to remove dnsmasq config file", slog.String("path", h.dnsmasqConfigPath), slog.String("error", err.Error()))
		} else {
			h.logger.Debug("removed dnsmasq config file", slog.String("path", h.dnsmasqConfigPath))
		}
		h.dnsmasqConfigPath = ""
	}
}

func (h *hostAPDService) enableIPForwarding() error {
	h.logger.Debug("enabling IP forwarding")

	// Enable IP forwarding
	if err := exec.Command("sudo", "sysctl", "-w", "net.ipv4.ip_forward=1").Run(); err != nil {
		return errors.Wrap(err, "failed to enable IP forwarding")
	}

	// Make it persistent across reboots (optional, for temporary testing can be skipped)
	if err := exec.Command("sudo", "sh", "-c", "echo 'net.ipv4.ip_forward=1' >> /etc/sysctl.conf").Run(); err != nil {
		h.logger.Debug("failed to make IP forwarding persistent", slog.String("error", err.Error()))
	}

	return nil
}

func (h *hostAPDService) cleanupNetworkRules() {
	h.logger.Debug("cleaning up network rules")

	// Clean up IPTables rules
	ipTablesRules := CleanupIPTablesRules(h.config.Interface, h.config.PortalPort)
	for _, rule := range ipTablesRules {
		if err := rule.Apply(); err != nil {
			h.logger.Debug("failed to remove iptables rule (may not exist)", slog.String("error", err.Error()))
		}
	}
}

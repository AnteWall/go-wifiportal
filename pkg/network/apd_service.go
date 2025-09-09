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
	CountryCode string `yaml:"country_code" json:"countryCode"`
	Security    string `yaml:"security" json:"security"` // "open", "wpa2"
	Gateway     string `yaml:"gateway" json:"gateway"`
	DHCPRange   string `yaml:"dhcp_range" json:"dhcpRange"`
	PortalPort  string `yaml:"portal_port" json:"portalPort"`
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
	h.logger.Info("starting access point service", slog.String("ssid", config.SSID))

	if err := h.prepareInterface(); err != nil {
		return errors.Wrap(err, "failed to prepare interface")
	}
	if err := h.createHotspot(); err != nil {
		return errors.Wrap(err, "failed to create NetworkManager hotspot")
	}
	if err := h.configureNetwork(); err != nil {
		return errors.Wrap(err, "failed to configure network")
	}
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
	h.logger.Debug("access point service stopped")
	return nil
}

func (h *hostAPDService) IsRunning() bool {
	return h.running
}

func (h *hostAPDService) prepareInterface() error {
	// Stop any existing dnsmasq service
	if err := exec.Command("systemctl", "stop", "dnsmasq").Run(); err != nil {
		h.logger.Warn("failed to stop system dnsmasq service", slog.String("error", err.Error()))
	}

	// Ensure the interface is managed by NetworkManager
	if err := exec.Command("nmcli", "device", "set", h.config.Interface, "managed", "yes").Run(); err != nil {
		return errors.Wrap(err, "failed to set interface to managed mode")
	}

	// Disconnect any existing connections on the interface
	if o, err := exec.Command("nmcli", "device", "disconnect", h.config.Interface).CombinedOutput(); err != nil {
		if !strings.Contains(string(o), "This device is not active") {
			h.logger.Warn("failed to disconnect interface", slog.String("error", err.Error()))
		}
	}

	return nil
}

func (h *hostAPDService) createHotspot() error {
	args := []string{
		"connection", "add",
		"type", "wifi",
		"ifname", h.config.Interface,
		"con-name", h.config.Name,
		"autoconnect", "yes",
		"wifi.mode", "ap",
		"wifi.ssid", h.config.SSID,
		"ipv4.method", "manual",
		"ipv4.addresses", fmt.Sprintf("%s/24", h.config.Gateway),
	}

	// Add security settings based on configuration
	if h.config.Security == "wpa2" && h.config.Password != "" {
		args = append(args,
			"wifi-sec.key-mgmt", "wpa-psk",
			"wifi-sec.proto", "rsn",
			"wifi-sec.pairwise", "ccmp",
			"wifi-sec.group", "ccmp",
			"wifi-sec.psk", h.config.Password,
		)
	} else if h.config.Password != "" {
		args = append(args,
			"wifi-sec.key-mgmt", "wpa-psk",
			"wifi-sec.proto", "rsn",
			"wifi-sec.pairwise", "ccmp",
			"wifi-sec.group", "ccmp",
			"wifi-sec.psk", h.config.Password,
		)
	}

	cmd := exec.Command("nmcli", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	}

	cmd = exec.Command("nmcli", "connection", "up", h.config.Name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to activate hotspot: %s, %w", string(output), err)
	}
	return nil
}

func (h *hostAPDService) configureNetwork() error {
	rules := GetRequiredFirewallRules(h.config.Interface, h.config.PortalPort)
	for _, rule := range rules {
		if err := rule.Apply(h.config.Interface); err != nil {
			h.logger.Warn("failed to apply firewall rule", slog.String("error", err.Error()))
		}
	}

	ipTablesRules := CreateIPTablesRules(h.config.Interface, h.config.PortalPort)
	for _, rule := range ipTablesRules {
		if err := rule.Apply(); err != nil {
			h.logger.Warn("failed to apply iptables rule", slog.String("error", err.Error()))
		}
	}

	return nil
}

func (h *hostAPDService) startDNSMasq() error {
	// Stop any existing dnsmasq service
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

	h.dnsmasqCmd = exec.Command("sudo", "dnsmasq", "-C", h.dnsmasqConfigPath, "--keep-in-foreground")
	if err := h.dnsmasqCmd.Start(); err != nil {
		return fmt.Errorf("failed to start dnsmasq: %w", err)
	}

	return nil
}

func (h *hostAPDService) stopHotspot() {
	if err := exec.Command("nmcli", "connection", "down", h.config.Name).Run(); err != nil {
		h.logger.Error("failed to disconnect hotspot", slog.String("name", h.config.Name), slog.String("error", err.Error()))
	}

	if err := exec.Command("nmcli", "connection", "delete", h.config.Name).Run(); err != nil {
		h.logger.Error("failed to delete hotspot connection", slog.String("name", h.config.Name), slog.String("error", err.Error()))
	}
}

func (h *hostAPDService) stopDNSMasq() {
	if h.dnsmasqCmd != nil && h.dnsmasqCmd.Process != nil {
		if err := h.dnsmasqCmd.Process.Kill(); err != nil {
			h.logger.Error("failed to kill dnsmasq process", slog.String("error", err.Error()))
		}
		h.dnsmasqCmd.Wait()
		h.dnsmasqCmd = nil
	}

	if h.dnsmasqConfigPath != "" {
		pattern := "dnsmasq.*" + h.dnsmasqConfigPath
		exec.Command("pkill", "-f", pattern).Run()

		if err := os.Remove(h.dnsmasqConfigPath); err != nil {
			h.logger.Error("failed to remove dnsmasq config file", slog.String("path", h.dnsmasqConfigPath), slog.String("error", err.Error()))
		}
		h.dnsmasqConfigPath = ""
	}
}

func (h *hostAPDService) cleanupNetworkRules() {
	ipTablesRules := CleanupIPTablesRules(h.config.Interface, h.config.PortalPort)
	for _, rule := range ipTablesRules {
		rule.Apply()
	}
}

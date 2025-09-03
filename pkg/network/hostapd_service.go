package network

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

var (
	DefaultCountryCode       = "US"
	ErrInvalidAPConfig       = errors.New("invalid wireless wireless access point")
	ErrServiceAlreadyRunning = errors.New("hostapd service is already running")
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
	Security    string `yaml:"security" json:"security"` // open, wpa2
	Gateway     string `yaml:"gateway" json:"gateway"`
	DHCPRange   string `yaml:"dhcp_range" json:"dhcpRange"`
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
	if c.Channel < 1 || c.Channel > 14 {
		return errors.Wrap(ErrInvalidAPConfig, "channel must be between 1 and 14")
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
	return nil
}

type HostAPDService interface {
	Start(ctx context.Context, config APConfig) error
	Stop(ctx context.Context) error
	IsRunning() bool
}

type hostAPDService struct {
	config     APConfig
	configPath string
	running    bool
	logger     *slog.Logger
}

func NewHostAPDService() HostAPDService {
	return &hostAPDService{
		logger:  slog.Default().WithGroup("hostapd_service"),
		running: false,
	}
}

func (h *hostAPDService) Start(ctx context.Context, config APConfig) error {
	if h.running {
		return ErrServiceAlreadyRunning
	}
	if err := h.config.Validate(); err != nil {
		return errors.Wrap(err, "invalid access point configuration")
	}
	h.config = config

	if err := h.prepareInterface(); err != nil {
		return errors.Wrap(err, "failed to prepare interface")
	}
	if err := h.generateHostapdConfig(); err != nil {
		return errors.Wrap(err, "failed to generate hostapd config")
	}
	if err := h.configureNetwork(); err != nil {
		return errors.Wrap(err, "failed to configure network")
	}
	if err := h.startDNSMasq(); err != nil {
		return errors.Wrap(err, "failed to start dnsmasq")
	}
	if err := h.startHostapd(); err != nil {
		return errors.Wrap(err, "failed to start hostapd")
	}

	h.running = true
	return nil
}

func (h *hostAPDService) Stop(ctx context.Context) error {
	if !h.running {
		return nil
	}

	h.stopHostapd()

	h.running = false
	return nil
}

func (h *hostAPDService) IsRunning() bool {
	return h.running
}

func (h *hostAPDService) prepareInterface() error {
	h.logger.Debug("preparing hostapd interface")

	h.logger.Debug("stopping dnsmasq service")

	err := exec.Command("systemctl", "stop", "dnsmasq").Run()
	if err != nil {
		return err
	}

	h.logger.Debug("setting interface to non managed mode")
	if err := exec.Command("nmcli", "device", "set", h.config.Interface, "managed", "no").Run(); err != nil {
		return errors.Wrap(err, "failed disable interface managed mode")
	}

	h.logger.Debug("resetting interface")
	if err := exec.Command("ip", "link", "set", h.config.Interface, "down").Run(); err != nil {
		return errors.Wrap(err, "failed to bring interface down")
	}
	if err := exec.Command("ip", "addr", "flush", "dev").Run(); err != nil {
		return errors.Wrap(err, "failed to flush interface addresses")
	}
	if err := exec.Command("iw", "dev", h.config.Interface, "set", "type", "managed").Run(); err != nil {
		return errors.Wrap(err, "failed to set interface type to managed")
	}
	if err := exec.Command("ip", "link", "set", h.config.Interface, "up").Run(); err != nil {
		return errors.Wrap(err, "failed to bring interface up")
	}
	if err := h.setRegulatoryDomain(); err != nil {
		return errors.Wrap(err, "failed to set regulatory domain")
	}
	h.logger.Debug("setting interface to AP mode")
	if err := exec.Command("iw", "dev", h.config.Interface, "set", "type", "__ap").Run(); err != nil {
		return errors.Wrap(err, "failed to set interface type to AP")
	}
	h.logger.Debug("bringing interface up")
	if err := exec.Command("ip", "link", "set", h.config.Interface, "up").Run(); err != nil {
		return errors.Wrap(err, "failed to bring interface up")
	}
	return h.verifyInterfaceStatus(h.config.Interface)
}

// setRegulatoryDomain sets the wireless regulatory domain
func (h *hostAPDService) setRegulatoryDomain() error {
	countryCode := h.config.CountryCode
	if countryCode == "" {
		countryCode = DefaultCountryCode
	}
	h.logger.Debug("setting regulatory domain", slog.String("country_code", countryCode))
	if err := exec.Command("iw", "reg", "set", countryCode).Run(); err != nil {
		return fmt.Errorf("failed to set regulatory domain to %s: %w", countryCode, err)
	}
	return nil
}

// verifyInterfaceStatus verifies the interface is properly configured
func (h *hostAPDService) verifyInterfaceStatus(iFace string) error {
	checkCmd := exec.Command("ip", "link", "show", iFace)
	output, err := checkCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to verify interface status: %w", err)
	}
	h.logger.Debug("interface status", slog.String("output", string(output)))
	return nil
}

func (h *hostAPDService) generateHostapdConfig() error {
	tmpl, err := template.New("hostapd").ParseFS(templateFiles, "templates/hostapd_config.tmpl")
	if err != nil {
		return errors.Wrap(err, "failed to parse hostapd template")
	}
	file, err := os.CreateTemp("hostapd", "hostapd.conf")
	if err != nil {
		return errors.Wrap(err, "failed to create hostapd config file")
	}
	defer file.Close()
	if err := tmpl.Execute(file, h.config); err != nil {
		return errors.Wrap(err, "failed to execute hostapd template")
	}
	h.configPath = file.Name()
	h.logger.Debug("generated hostapd config", slog.String("path", h.configPath))
	return nil

}

func (h *hostAPDService) configureNetwork() error {
	if err := exec.Command("ip", "addr", "add", fmt.Sprintf("%s/24", h.config.Gateway), "dev", h.config.Interface).Run(); err != nil {
		return errors.Wrap(err, "failed to configure network")
	}
	rules := GetRequiredFirewallRules(h.config.Interface)
	for _, rule := range rules {
		if err := rule.Apply(h.config.Interface); err != nil {
			return errors.Wrap(err, "failed to apply firewall rule")
		}
	}
	ipTablesRules := CreateIPTablesRules(h.config.Interface, h.config.Gateway)
	for _, rule := range ipTablesRules {
		if err := rule.Apply(); err != nil {
			return errors.Wrap(err, "failed to apply iptables rule")
		}
	}
	return nil
}

func (h *hostAPDService) startDNSMasq() error {
	tmpl, err := template.New("dnsmasq").ParseFS(templateFiles, "templates/dnsmasq_config.tmpl")
	if err != nil {
		return errors.Wrap(err, "failed to parse hostapd template")
	}
	file, err := os.CreateTemp("dnsmasq", "dnsmasq.conf")
	if err != nil {
		return errors.Wrap(err, "failed to create hostapd config file")
	}
	defer file.Close()
	if err := tmpl.Execute(file, h.config); err != nil {
		return errors.Wrap(err, "failed to execute hostapd template")
	}

	cmd := exec.Command("dnsmasq", "-C", file.Name(), "-d")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start dnsmasq: %s, %w", string(output), err)
	}
	h.logger.Debug("started dnsmasq service")
	return nil
}

func (h *hostAPDService) startHostapd() error {
	h.logger.Debug("starting hostapd service")
	cmd := exec.Command("hostapd", h.configPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start hostapd: %s, %w", string(output), err)
	}
	h.logger.Debug("started hostapd service")
	return nil
}

func (h *hostAPDService) stopHostapd() {
	pattern := "hostapd.*" + h.configPath
	h.logger.Debug("stopping hostapd service")
	if err := exec.Command("pkill", "-f", pattern).Run(); err != nil {
		h.logger.Error("failed to stop hostapd", slog.String("pattern", pattern), slog.String("error", err.Error()))
	} else {
		h.logger.Debug("stopped hostapd service")
	}
	// Cleanup config file
	if err := os.Remove(h.configPath); err != nil {
		h.logger.Error("failed to remove hostapd config file", slog.String("path", h.configPath), slog.String("error", err.Error()))
	} else {
		h.logger.Debug("removed hostapd config file", slog.String("path", h.configPath))
	}
	return
}

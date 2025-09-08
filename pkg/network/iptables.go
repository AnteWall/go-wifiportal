package network

import (
	"log/slog"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

type IPTablesRule struct {
	args []string
}

func NewIPTablesRule(args ...string) IPTablesRule {
	return IPTablesRule{args: args}
}

func (r IPTablesRule) Apply() error {
	args := append([]string{"iptables-legacy"}, r.args...)
	if i, err := exec.Command("sudo", args...).CombinedOutput(); err != nil {
		slog.Error(strings.Join(args, " "), slog.String("output", string(i)), slog.String("error", err.Error()))
		return errors.Wrap(err, string(i))
	}
	return nil
}

// getDefaultInterface returns the default network interface for internet access
func getDefaultInterface() string {
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		slog.Warn("failed to get default interface", slog.String("error", err.Error()))
		return "eth0" // fallback
	}

	// Parse "default via X.X.X.X dev ethX ..."
	parts := strings.Fields(string(output))
	for i, part := range parts {
		if part == "dev" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return "eth0" // fallback
}

func CreateIPTablesRules(iFace, portalPort string) []IPTablesRule {
	defaultIface := getDefaultInterface()

	return []IPTablesRule{
		// Enable IP forwarding first
		NewIPTablesRule("-P", "FORWARD", "ACCEPT"),

		// Redirect all client HTTP traffic (80) to local portalPort
		NewIPTablesRule("-t", "nat", "-A", "PREROUTING",
			"-i", iFace, "-p", "tcp", "--dport", "80",
			"-j", "REDIRECT", "--to-ports", portalPort),

		// Allow clients to reach the portal service
		NewIPTablesRule("-A", "INPUT",
			"-i", iFace, "-p", "tcp", "--dport", portalPort, "-j", "ACCEPT"),

		// Allow DHCP traffic
		NewIPTablesRule("-A", "INPUT",
			"-i", iFace, "-p", "udp", "--dport", "67", "-j", "ACCEPT"),
		NewIPTablesRule("-A", "INPUT",
			"-i", iFace, "-p", "udp", "--dport", "53", "-j", "ACCEPT"),
		NewIPTablesRule("-A", "INPUT",
			"-i", iFace, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"),

		// Let guest traffic be forwarded out
		NewIPTablesRule("-A", "FORWARD", "-i", iFace, "-j", "ACCEPT"),
		NewIPTablesRule("-A", "FORWARD", "-o", iFace, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"),

		// NAT/masquerade for internet access
		NewIPTablesRule("-t", "nat", "-A", "POSTROUTING", "-o", defaultIface, "-j", "MASQUERADE"),
	}
}

func CleanupIPTablesRules(iFace, portalPort string) []IPTablesRule {
	defaultIface := getDefaultInterface()

	return []IPTablesRule{
		NewIPTablesRule("-t", "nat", "-D", "PREROUTING",
			"-i", iFace, "-p", "tcp", "--dport", "80",
			"-j", "REDIRECT", "--to-ports", portalPort),

		NewIPTablesRule("-D", "INPUT",
			"-i", iFace, "-p", "tcp", "--dport", portalPort, "-j", "ACCEPT"),

		NewIPTablesRule("-D", "INPUT",
			"-i", iFace, "-p", "udp", "--dport", "67", "-j", "ACCEPT"),
		NewIPTablesRule("-D", "INPUT",
			"-i", iFace, "-p", "udp", "--dport", "53", "-j", "ACCEPT"),
		NewIPTablesRule("-D", "INPUT",
			"-i", iFace, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"),

		NewIPTablesRule("-D", "FORWARD", "-i", iFace, "-j", "ACCEPT"),
		NewIPTablesRule("-D", "FORWARD", "-o", iFace, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"),

		NewIPTablesRule("-t", "nat", "-D", "POSTROUTING", "-o", defaultIface, "-j", "MASQUERADE"),
	}
}

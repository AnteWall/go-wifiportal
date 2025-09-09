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

func CreateIPTablesRules(iFace, portalPort string) []IPTablesRule {
	return []IPTablesRule{
		// Redirect all client HTTP traffic (80) to local portal server
		NewIPTablesRule("-t", "nat", "-A", "PREROUTING",
			"-i", iFace, "-p", "tcp", "--dport", "80",
			"-j", "REDIRECT", "--to-ports", portalPort),

		// Allow clients to reach the portal service
		NewIPTablesRule("-A", "INPUT",
			"-i", iFace, "-p", "tcp", "--dport", portalPort, "-j", "ACCEPT"),

		// Allow DHCP and DNS traffic for local network
		NewIPTablesRule("-A", "INPUT",
			"-i", iFace, "-p", "udp", "--dport", "67", "-j", "ACCEPT"),
		NewIPTablesRule("-A", "INPUT",
			"-i", iFace, "-p", "udp", "--dport", "53", "-j", "ACCEPT"),
		NewIPTablesRule("-A", "INPUT",
			"-i", iFace, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"),
	}
}

func CleanupIPTablesRules(iFace, portalPort string) []IPTablesRule {
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
	}
}

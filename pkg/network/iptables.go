package network

import (
	"fmt"
	"os/exec"

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
		return errors.Wrap(err, string(i))
	}
	return nil
}

func CreateIPTablesRules(iFace string, gateway string) []IPTablesRule {
	return []IPTablesRule{
		NewIPTablesRule("-t", "nat", "-A", "PREROUTING", "-i", iFace, "-p", "tcp", "--dport", "80", "-j", "DNAT", "--to-destination", fmt.Sprintf("%s:80", gateway)),
		NewIPTablesRule("-t", "nat", "-A", "PREROUTING", "-i", iFace, "-p", "tcp", "--dport", "443", "-j", "DNAT", "--to-destination", fmt.Sprintf("%s:80", gateway)),
		NewIPTablesRule("-A", "INPUT", "-i", iFace, "-p", "tcp", "--dport", "80", "-j", "ACCEPT"),
		NewIPTablesRule("-A", "FORWARD", "-i", iFace, "-j", "ACCEPT"),
	}
}

// CleanupIPTablesRules removes the rules (useful for cleanup)
func CleanupIPTablesRules(iFace string, gateway string) []IPTablesRule {
	return []IPTablesRule{
		// Remove NAT rules
		NewIPTablesRule("-t", "nat", "-D", "PREROUTING", "-i", iFace, "-p", "tcp", "--dport", "80", "-j", "DNAT", "--to-destination", fmt.Sprintf("%s:80", gateway)),
		NewIPTablesRule("-t", "nat", "-D", "PREROUTING", "-i", iFace, "-p", "tcp", "--dport", "443", "-j", "DNAT", "--to-destination", fmt.Sprintf("%s:80", gateway)),

		// Remove filter rules
		NewIPTablesRule("-D", "INPUT", "-i", iFace, "-p", "tcp", "--dport", "80", "-j", "ACCEPT"),
		NewIPTablesRule("-D", "FORWARD", "-i", iFace, "-j", "ACCEPT"),
	}
}

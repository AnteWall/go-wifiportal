package network

import (
	"fmt"
	"os/exec"
)

type IPTablesRule struct {
	args []string
}

func NewIPTablesRule(args ...string) IPTablesRule {
	return IPTablesRule{args: args}
}

func (r IPTablesRule) Apply() error {
	return exec.Command("iptables", r.args...).Run()
}

func CreateIPTablesRules(iFace string, gateway string) []IPTablesRule {
	return []IPTablesRule{
		NewIPTablesRule("-t", "nat", "-A", "PREROUTING", "-i", iFace, "-p", "tcp", "--dport", "80", "-j", "DNAT", "--to-destination", fmt.Sprintf("%s:80", gateway)),
		NewIPTablesRule("-t", "nat", "-A", "PREROUTING", "-i", iFace, "-p", "tcp", "--dport", "443", "-j", "DNAT", "--to-destination", fmt.Sprintf("%s:80", gateway)),
		NewIPTablesRule("-A", "INPUT", "-i", iFace, "-p", "tcp", "--dport", "80", "-j", "ACCEPT"),
		NewIPTablesRule("-A", "FORWARD", "-i", iFace, "-j", "ACCEPT"),
	}
}

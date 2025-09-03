package network

import "os/exec"

type FireWallDirection int

const (
	INCOMING FireWallDirection = iota
	OUTGOING
	BOTH
)

func (d FireWallDirection) String() string {
	switch d {
	case INCOMING:
		return "in"
	case OUTGOING:
		return "out"
	case BOTH:
		return "in out"
	default:
		return ""
	}
}

type FireWallProtocol int

const (
	TCP FireWallProtocol = iota
	UDP
	ANY
)

func (p FireWallProtocol) ToString() string {
	switch p {
	case TCP:
		return "tcp"
	case UDP:
		return "udp"
	case ANY:
		return "any"
	default:
		return "any"
	}

}

type FireWallRule struct {
	Direction FireWallDirection
	Interface string
	Port      string
	Protocol  FireWallProtocol
}

func (p FireWallRule) ToArgs(iFace string) []string {
	return []string{"allow", p.Direction.String(), "on", iFace, "to any", "port", p.Port, "proto", p.Protocol.ToString()}
}

func (p FireWallRule) Apply(iFace string) error {
	return exec.Command("uwf", p.ToArgs(iFace)...).Run()
}

func GetRequiredFirewallRules(iFace string) []FireWallRule {
	return []FireWallRule{
		{Direction: INCOMING, Interface: iFace, Port: "67", Protocol: UDP},
		{Direction: OUTGOING, Interface: iFace, Port: "68", Protocol: UDP},
		{Direction: INCOMING, Interface: iFace, Port: "80", Protocol: TCP},
		{Direction: INCOMING, Interface: iFace, Port: "53", Protocol: UDP},
		{Direction: INCOMING, Interface: iFace, Port: "53", Protocol: TCP},
	}
}

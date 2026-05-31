package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

const firewallPrefix = "DATN-DEMO"

func listFirewallRules() ([]FirewallRule, error) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}
	out, err := exec.Command("netsh", "advfirewall", "firewall", "show", "rule", "name=all").CombinedOutput()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	var rules []FirewallRule
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Rule Name:") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "Rule Name:"))
		if strings.HasPrefix(name, firewallPrefix) {
			rules = append(rules, FirewallRule{Name: name})
		}
	}
	return rules, nil
}

func healAllFirewallRules() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	rules, _ := listFirewallRules()
	for _, rule := range rules {
		_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+rule.Name).Run()
	}
	return nil
}

func applyFirewallPartition(instances []DemoInstanceProfile, spec PartitionSpec) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("Windows firewall partitioning is only available on Windows")
	}
	if len(spec.Clusters) < 2 {
		return fmt.Errorf("partition requires at least two clusters")
	}
	if err := healAllFirewallRules(); err != nil {
		return err
	}
	byID := make(map[string]DemoInstanceProfile, len(instances))
	for _, inst := range instances {
		byID[inst.ID] = inst
	}
	for i := 0; i < len(spec.Clusters); i++ {
		for j := i + 1; j < len(spec.Clusters); j++ {
			for _, a := range spec.Clusters[i] {
				for _, b := range spec.Clusters[j] {
					left, okLeft := byID[a]
					right, okRight := byID[b]
					if !okLeft || !okRight {
						return fmt.Errorf("unknown instance in partition: %s/%s", a, b)
					}
					if err := blockPair(spec.ID, left, right); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func blockPair(partitionID string, a DemoInstanceProfile, b DemoInstanceProfile) error {
	nameBase := fmt.Sprintf("%s-%s-%s-%s", firewallPrefix, sanitizeRule(partitionID), a.ID, b.ID)
	rules := [][]string{
		// Outbound TCP/UDP blocks
		{"dir=out", fmt.Sprintf("localip=%s", a.BindIP), fmt.Sprintf("remoteport=%d", b.P2PPort), "protocol=TCP"},
		{"dir=out", fmt.Sprintf("localip=%s", b.BindIP), fmt.Sprintf("remoteport=%d", a.P2PPort), "protocol=TCP"},
		{"dir=out", fmt.Sprintf("localip=%s", a.BindIP), fmt.Sprintf("remoteport=%d", b.P2PPort), "protocol=UDP"},
		{"dir=out", fmt.Sprintf("localip=%s", b.BindIP), fmt.Sprintf("remoteport=%d", a.P2PPort), "protocol=UDP"},

		// Inbound TCP/UDP blocks
		{"dir=in", fmt.Sprintf("localip=%s", a.BindIP), fmt.Sprintf("localport=%d", a.P2PPort), fmt.Sprintf("remoteip=%s", b.BindIP), "protocol=TCP"},
		{"dir=in", fmt.Sprintf("localip=%s", b.BindIP), fmt.Sprintf("localport=%d", b.P2PPort), fmt.Sprintf("remoteip=%s", a.BindIP), "protocol=TCP"},
		{"dir=in", fmt.Sprintf("localip=%s", a.BindIP), fmt.Sprintf("localport=%d", a.P2PPort), fmt.Sprintf("remoteip=%s", b.BindIP), "protocol=UDP"},
		{"dir=in", fmt.Sprintf("localip=%s", b.BindIP), fmt.Sprintf("localport=%d", b.P2PPort), fmt.Sprintf("remoteip=%s", a.BindIP), "protocol=UDP"},
	}
	for i, parts := range rules {
		args := []string{"advfirewall", "firewall", "add", "rule", "name=" + fmt.Sprintf("%s-%d", nameBase, i+1), "action=block"}
		args = append(args, parts...)
		if out, err := exec.Command("netsh", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("add firewall rule: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func sanitizeRule(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return "partition"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", ".", "-")
	return replacer.Replace(in)
}

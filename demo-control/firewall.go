package main

import (
	"fmt"
	"os/exec"
	"strings"
)

const firewallPrefix = "DATN-DEMO"

func listFirewallRules() ([]FirewallRule, error) {
	var rules []FirewallRule
	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("node-%02d", i)
		if !isContainerRunning(id) {
			continue
		}
		out, err := exec.Command("docker", "exec", "-u", "root", id, "iptables", "-S").CombinedOutput()
		if err != nil {
			continue
		}
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "-j DROP") {
				// Format: -A INPUT -s 10.20.30.12 -j DROP
				parts := strings.Fields(line)
				srcIP := ""
				for idx, part := range parts {
					if part == "-s" && idx+1 < len(parts) {
						srcIP = parts[idx+1]
						break
					}
				}
				if srcIP != "" {
					targetNode := nodeIDFromIP(srcIP)
					if targetNode != "" {
						rules = append(rules, FirewallRule{
							Name: fmt.Sprintf("%s-%s-%s", firewallPrefix, id, targetNode),
						})
					} else {
						rules = append(rules, FirewallRule{
							Name: fmt.Sprintf("%s-%s-block-%s", firewallPrefix, id, srcIP),
						})
					}
				}
			}
		}
	}
	return rules, nil
}

func nodeIDFromIP(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		var lastByte int
		if _, err := fmt.Sscanf(parts[3], "%d", &lastByte); err == nil {
			if lastByte >= 11 && lastByte <= 20 {
				return fmt.Sprintf("node-%02d", lastByte-10)
			}
		}
	}
	return ""
}

func healAllFirewallRules() error {
	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("node-%02d", i)
		if isContainerRunning(id) {
			_ = exec.Command("docker", "exec", "-u", "root", id, "iptables", "-F").Run()
		}
	}
	return nil
}

func applyFirewallPartition(instances []DemoInstanceProfile, spec PartitionSpec) error {
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
					if isContainerRunning(left.ID) && isContainerRunning(right.ID) {
						if err := blockPair(spec.ID, left, right); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

func blockPair(partitionID string, a DemoInstanceProfile, b DemoInstanceProfile) error {
	// Add DROP rule in container a for packets coming from container b's IP
	cmd1 := exec.Command("docker", "exec", "-u", "root", a.ID, "iptables", "-A", "INPUT", "-s", b.BindIP, "-j", "DROP")
	if out, err := cmd1.CombinedOutput(); err != nil {
		return fmt.Errorf("blockPair node %s -> %s failed: %w: %s", a.ID, b.ID, err, string(out))
	}
	// Add DROP rule in container b for packets coming from container a's IP
	cmd2 := exec.Command("docker", "exec", "-u", "root", b.ID, "iptables", "-A", "INPUT", "-s", a.BindIP, "-j", "DROP")
	if out, err := cmd2.CombinedOutput(); err != nil {
		return fmt.Errorf("blockPair node %s -> %s failed: %w: %s", b.ID, a.ID, err, string(out))
	}
	return nil
}

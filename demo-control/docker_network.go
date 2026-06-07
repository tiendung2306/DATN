package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const (
	sharedDockerNetwork    = "datn_p2p_net"
	sharedDockerSubnet     = "10.20.30.0/24"
	partitionNetworkPrefix = "datn_p2p_part_"
	partitionSubnetBase    = "10.90"
)

type dockerInspectRecord struct {
	Name            string `json:"Name"`
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string `json:"IPAddress"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

type dockerNetworkInspectRecord struct {
	Name       string `json:"Name"`
	Containers map[string]struct {
		Name        string `json:"Name"`
		IPv4Address string `json:"IPv4Address"`
	} `json:"Containers"`
}

func (a *App) ensureSharedNetwork() error {
	return ensureDockerNetwork(sharedDockerNetwork, sharedDockerSubnet)
}

func (a *App) ensurePartitionNetwork(name string, index int) error {
	subnet := fmt.Sprintf("%s.%d.0/24", partitionSubnetBase, index+1)
	return ensureDockerNetwork(name, subnet)
}

func ensureDockerNetwork(name string, subnet string) error {
	if dockerNetworkExists(name) {
		return nil
	}
	args := []string{"network", "create"}
	if subnet != "" {
		args = append(args, "--subnet", subnet)
	}
	args = append(args, name)
	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("create docker network %s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func dockerNetworkExists(name string) bool {
	out, err := exec.Command("docker", "network", "inspect", name).CombinedOutput()
	return err == nil && len(out) > 0
}

func (a *App) listContainerNetworks(container string) ([]string, error) {
	out, err := exec.Command("docker", "inspect", container).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("inspect container %s: %w (%s)", container, err, strings.TrimSpace(string(out)))
	}

	var records []dockerInspectRecord
	if err := json.Unmarshal(out, &records); err != nil {
		return nil, fmt.Errorf("decode container inspect %s: %w", container, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("container %s not found", container)
	}

	networks := make([]string, 0, len(records[0].NetworkSettings.Networks))
	for name := range records[0].NetworkSettings.Networks {
		networks = append(networks, name)
	}
	sort.Strings(networks)
	return networks, nil
}

func (a *App) connectContainerToNetwork(container string, network string, ipAddress string) error {
	networks, err := a.listContainerNetworks(container)
	if err == nil {
		for _, existing := range networks {
			if existing == network {
				return nil
			}
		}
	}

	args := []string{"network", "connect"}
	if strings.TrimSpace(ipAddress) != "" {
		args = append(args, "--ip", strings.TrimSpace(ipAddress))
	}
	args = append(args, network, container)
	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("connect %s to %s: %w (%s)", container, network, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *App) disconnectContainerFromNetwork(container string, network string) error {
	networks, err := a.listContainerNetworks(container)
	if err == nil {
		found := false
		for _, existing := range networks {
			if existing == network {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	out, err := exec.Command("docker", "network", "disconnect", "-f", network, container).CombinedOutput()
	if err != nil {
		return fmt.Errorf("disconnect %s from %s: %w (%s)", container, network, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *App) healContainerToSharedNetwork(profile DemoInstanceProfile) error {
	if profile.LaunchMode != "docker" {
		return nil
	}
	if !isContainerRunning(profile.ID) {
		return nil
	}
	if err := a.ensureSharedNetwork(); err != nil {
		return err
	}
	if err := a.connectContainerToNetwork(profile.ID, sharedDockerNetwork, profile.BindIP); err != nil {
		return err
	}
	networks, err := a.listContainerNetworks(profile.ID)
	if err != nil {
		return err
	}
	for _, network := range networks {
		if network == sharedDockerNetwork {
			continue
		}
		if isManagedPartitionNetwork(network) {
			if err := a.disconnectContainerFromNetwork(profile.ID, network); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *App) moveContainerToPartitionNetwork(profile DemoInstanceProfile, network string, ipAddress string) error {
	if profile.LaunchMode != "docker" {
		return fmt.Errorf("network partition is only supported for docker-managed instances")
	}
	if !isContainerRunning(profile.ID) {
		return nil
	}
	if err := a.connectContainerToNetwork(profile.ID, network, ipAddress); err != nil {
		return err
	}
	networks, err := a.listContainerNetworks(profile.ID)
	if err != nil {
		return err
	}
	for _, attached := range networks {
		if attached == network {
			continue
		}
		if attached == sharedDockerNetwork || isManagedPartitionNetwork(attached) {
			if err := a.disconnectContainerFromNetwork(profile.ID, attached); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *App) cleanupUnusedPartitionNetworks() {
	names, err := listManagedPartitionNetworks()
	if err != nil {
		return
	}
	for _, name := range names {
		if inUse, err := dockerNetworkInUse(name); err == nil && !inUse {
			_, _ = exec.Command("docker", "network", "rm", name).CombinedOutput()
		}
	}
}

func listManagedPartitionNetworks() ([]string, error) {
	out, err := exec.Command("docker", "network", "ls", "--format", "{{.Name}}").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list docker networks: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	lines := strings.Split(string(out), "\n")
	var names []string
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if isManagedPartitionNetwork(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func dockerNetworkInUse(name string) (bool, error) {
	out, err := exec.Command("docker", "network", "inspect", name).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("inspect docker network %s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	var records []dockerNetworkInspectRecord
	if err := json.Unmarshal(out, &records); err != nil {
		return false, fmt.Errorf("decode docker network inspect %s: %w", name, err)
	}
	if len(records) == 0 {
		return false, nil
	}
	return len(records[0].Containers) > 0, nil
}

func (a *App) getNetworkTopologyForProfiles(profiles []DemoInstanceProfile) NetworkTopology {
	topology := NetworkTopology{
		SharedNetwork: sharedDockerNetwork,
	}

	partitionMap := make(map[string][]string)
	for _, profile := range profiles {
		nodeState := NodeNetworkState{
			NodeID:          profile.ID,
			IsDockerManaged: profile.LaunchMode == "docker",
		}

		if profile.LaunchMode != "docker" || !isContainerRunning(profile.ID) {
			topology.NodeNetworks = append(topology.NodeNetworks, nodeState)
			continue
		}

		networks, err := a.listContainerNetworks(profile.ID)
		if err != nil {
			topology.NodeNetworks = append(topology.NodeNetworks, nodeState)
			continue
		}
		nodeState.Networks = networks
		nodeState.PrimaryNetwork = primaryManagedNetwork(networks)
		if nodeState.PrimaryNetwork == "" && len(networks) > 0 {
			nodeState.PrimaryNetwork = networks[0]
		}
		for _, network := range networks {
			if isManagedPartitionNetwork(network) {
				partitionMap[network] = append(partitionMap[network], profile.ID)
			}
		}
		topology.NodeNetworks = append(topology.NodeNetworks, nodeState)
	}

	sort.Slice(topology.NodeNetworks, func(i, j int) bool {
		return topology.NodeNetworks[i].NodeID < topology.NodeNetworks[j].NodeID
	})

	if len(partitionMap) == 0 {
		return topology
	}

	topology.Active = true
	names := make([]string, 0, len(partitionMap))
	for name := range partitionMap {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		nodes := append([]string(nil), partitionMap[name]...)
		sort.Strings(nodes)
		topology.PartitionNetworks = append(topology.PartitionNetworks, PartitionNetwork{
			Name:  name,
			Nodes: nodes,
		})
	}
	return topology
}

func isManagedPartitionNetwork(name string) bool {
	return strings.HasPrefix(name, partitionNetworkPrefix)
}

func primaryManagedNetwork(networks []string) string {
	for _, network := range networks {
		if isManagedPartitionNetwork(network) {
			return network
		}
	}
	for _, network := range networks {
		if network == sharedDockerNetwork {
			return network
		}
	}
	return ""
}

func partitionNetworkName(index int) string {
	return fmt.Sprintf("%s%d", partitionNetworkPrefix, index+1)
}

func partitionIPAddress(profile DemoInstanceProfile, index int) string {
	parts := strings.Split(profile.BindIP, ".")
	lastOctet := 10 + index
	if len(parts) == 4 {
		if parsed, err := strconv.Atoi(parts[3]); err == nil && parsed > 1 && parsed < 255 {
			lastOctet = parsed
		}
	}
	return fmt.Sprintf("%s.%d.%d", partitionSubnetBase, index+1, lastOctet)
}

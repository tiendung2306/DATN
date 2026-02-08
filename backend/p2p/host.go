package p2p

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/multiformats/go-multiaddr"
)

type P2PNode struct {
	Host    host.Host
	DHT     *dht.IpfsDHT
	PubSub  *pubsub.PubSub
	service *mdnsService
}

type mdnsService struct {
	h host.Host
}

func (m *mdnsService) HandlePeerFound(pi peer.AddrInfo) {
	slog.Info("mDNS found peer", "peer", pi.ID.String())
	if err := m.h.Connect(context.Background(), pi); err != nil {
		slog.Warn("Failed to connect to mDNS peer", "peer", pi.ID.String(), "error", err)
	}
}

func NewP2PNode(ctx context.Context, privKey crypto.PrivKey, listenPort int) (*P2PNode, error) {
	bestIP := GetBestLocalIP()
	slog.Info("Selected best network interface for P2P", "ip", bestIP)

	// 1. Initialize Libp2p Host
	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/%s/tcp/%d", bestIP, listenPort),      // TCP
			fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", bestIP, listenPort), // QUIC
		),
		libp2p.DefaultTransports,
		libp2p.DefaultSecurity,
		libp2p.DefaultMuxers,
		libp2p.NATPortMap(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	slog.Info("Libp2p Host started", "id", h.ID().String(), "addrs", h.Addrs())

	// 2. Initialize Kademlia DHT
	kademliaDHT, err := dht.New(ctx, h, dht.Mode(dht.ModeAuto))
	if err != nil {
		return nil, fmt.Errorf("failed to create DHT: %w", err)
	}

	if err := kademliaDHT.Bootstrap(ctx); err != nil {
		return nil, fmt.Errorf("failed to bootstrap DHT: %w", err)
	}

	// 3. Initialize GossipSub
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("failed to create GossipSub: %w", err)
	}

	node := &P2PNode{
		Host:   h,
		DHT:    kademliaDHT,
		PubSub: ps,
	}

	// 4. Initialize mDNS
	mService := &mdnsService{h: h}
	node.service = mService
	ser := mdns.NewMdnsService(h, "secure-p2p-discovery", mService)
	if err := ser.Start(); err != nil {
		slog.Warn("mDNS failed to start. Local discovery might be limited.", "error", err)
	} else {
		slog.Info("mDNS service started successfully", "interface", bestIP)
	}

	return node, nil
}

func (n *P2PNode) ConnectToPeer(ctx context.Context, addrStr string) error {
	addr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return fmt.Errorf("invalid multiaddr: %w", err)
	}

	info, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return fmt.Errorf("failed to get addr info: %w", err)
	}

	if err := n.Host.Connect(ctx, *info); err != nil {
		return fmt.Errorf("failed to connect to peer: %w", err)
	}

	slog.Info("Successfully connected to bootstrap peer", "peer", info.ID.String())
	return nil
}

func (n *P2PNode) Close() error {
	return n.Host.Close()
}

// GetBestLocalIP tries to find the best outbound IP using UDP trick, 
// and falls back to interface scanning if offline.
func GetBestLocalIP() string {
	// 1. Try UDP trick (requires internet or a route to 8.8.8.8)
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 1*time.Second)
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		ip := localAddr.IP.String()
		if ip != "" && ip != "0.0.0.0" {
			slog.Debug("Found best IP via UDP trick", "ip", ip)
			return ip
		}
	}

	// 2. Fallback: Scan interfaces (works offline)
	slog.Debug("UDP trick failed or timed out, falling back to interface scanning")
	ifaces, err := net.Interfaces()
	if err != nil {
		return "0.0.0.0"
	}

	for _, iface := range ifaces {
		// Skip down, loopback, and non-multicast interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}

		// Filter out common virtual interface names
		name := strings.ToLower(iface.Name)
		if strings.Contains(name, "docker") || strings.Contains(name, "veth") || 
		   strings.Contains(name, "wsl") || strings.Contains(name, "virtual") ||
		   strings.Contains(name, "vmware") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			ip = ip.To4()
			if ip != nil {
				slog.Debug("Found best IP via interface scanning", "ip", ip.String(), "interface", iface.Name)
				return ip.String()
			}
		}
	}

	return "0.0.0.0"
}


package p2p

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"app/admin"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/multiformats/go-multiaddr"
)

// AppDHTProtocolPrefix is the Kademlia application protocol prefix for this app.
// It must not be the default "/ipfs": with that prefix, go-libp2p-kad-dht enforces
// that the record validator map contains exactly "pk" and "ipns" — adding our
// "app" namespace would fail validation. A custom prefix skips that check.
const AppDHTProtocolPrefix = protocol.ID("/datn")

// appDHTValidator accepts all values stored under the "/app/" namespace.
// Data integrity is handled by MLS signatures — DHT just acts as a store.
type appDHTValidator struct{}

func (appDHTValidator) Validate(_ string, _ []byte) error { return nil }
func (appDHTValidator) Select(_ string, vals [][]byte) (int, error) {
	if len(vals) == 0 {
		return 0, fmt.Errorf("no values to select")
	}
	return 0, nil
}

type P2PNode struct {
	Host         host.Host
	DHT          *dht.IpfsDHT
	PubSub       *pubsub.PubSub
	AuthProtocol *AuthProtocol
	mdnsService  mdns.Service
}

type mdnsNotifee struct {
	h host.Host
}

func (m *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	slog.Info("mDNS found peer", "peer", pi.ID.String())
	if err := m.h.Connect(context.Background(), pi); err != nil {
		slog.Warn("Failed to connect to mDNS peer", "peer", pi.ID.String(), "error", err)
	}
}

// NewP2PNode creates and starts a fully-configured P2P node.
//
// localToken and rootPubKey are required for authenticated networks.
// When both are provided, the node will:
//   - Install an AuthGater to block blacklisted peers
//   - Run the /app/auth/1.0.0 handshake on every new connection
func NewP2PNode(
	ctx context.Context,
	privKey crypto.PrivKey,
	listenPort int,
	localToken *admin.InvitationToken,
	rootPubKey []byte,
) (*P2PNode, error) {
	bestIP := GetBestLocalIP()
	slog.Info("Selected best network interface for P2P", "ip", bestIP)

	// 1. Initialize AuthGater (always present; blacklist is empty at start)
	gater := NewAuthGater()

	// 2. Initialize Libp2p Host
	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/%s/tcp/%d", bestIP, listenPort),
			fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", bestIP, listenPort),
		),
		libp2p.DefaultTransports,
		libp2p.DefaultSecurity,
		libp2p.DefaultMuxers,
		libp2p.NATPortMap(),
		libp2p.ConnectionGater(gater),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}
	slog.Info("Libp2p Host started", "id", h.ID().String(), "addrs", h.Addrs())

	// 3. Initialize Kademlia DHT + "/app/" record namespace for KP / Welcome storage.
	kademliaDHT, err := dht.New(ctx, h,
		dht.Mode(dht.ModeAutoServer),
		dht.ProtocolPrefix(AppDHTProtocolPrefix),
		dht.NamespacedValidator("app", appDHTValidator{}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create DHT: %w", err)
	}
	if err := kademliaDHT.Bootstrap(ctx); err != nil {
		return nil, fmt.Errorf("failed to bootstrap DHT: %w", err)
	}

	// 4. Initialize GossipSub
	ps, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("failed to create GossipSub: %w", err)
	}

	node := &P2PNode{
		Host:   h,
		DHT:    kademliaDHT,
		PubSub: ps,
	}

	// 5. Initialize auth protocol (when token and root key are provided)
	if localToken != nil && len(rootPubKey) > 0 {
		ap := NewAuthProtocol(h, gater, localToken, rootPubKey)
		node.AuthProtocol = ap
		h.Network().Notify(&authNetworkNotifee{ap: ap})
		slog.Info("Auth protocol registered", "protocol", AuthProtocolID)
	} else {
		slog.Warn("No auth token provided — running WITHOUT peer authentication (dev mode)")
	}

	// 6. Initialize mDNS
	ser := mdns.NewMdnsService(h, "secure-p2p-discovery", &mdnsNotifee{h: h})
	if err := ser.Start(); err != nil {
		slog.Warn("mDNS failed to start. Local discovery might be limited.", "error", err)
	} else {
		node.mdnsService = ser
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
	if n.mdnsService != nil {
		n.mdnsService.Close()
	}
	if err := n.DHT.Close(); err != nil {
		slog.Warn("Failed to close DHT", "error", err)
	}
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


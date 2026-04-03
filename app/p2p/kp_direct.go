package p2p

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// KPOfferProtocol — creator opens a stream to the invitee and requests the
// public KeyPackage bytes. Works when both peers have a live connection (Noise)
// even if the DHT routing table is empty (typical on small LANs).
const KPOfferProtocol = protocol.ID("/app/kp-offer/1.0.0")

const maxKPPayload = 256 * 1024 // 256 KiB

// FetchKeyPackageDirect requests public KeyPackage bytes from target over a
// dedicated stream. Does not use the DHT.
func FetchKeyPackageDirect(ctx context.Context, h host.Host, target peer.ID) ([]byte, error) {
	if h.Network().Connectedness(target) != network.Connected {
		return nil, fmt.Errorf("peer %s is not connected — open a connection first (same LAN / bootstrap)", target)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	s, err := h.NewStream(ctx, target, KPOfferProtocol)
	if err != nil {
		return nil, fmt.Errorf("kp-offer stream: %w", err)
	}
	defer s.Close()

	if _, err := s.Write([]byte{0x01}); err != nil {
		return nil, err
	}

	var lb [4]byte
	if _, err := io.ReadFull(s, lb[:]); err != nil {
		return nil, fmt.Errorf("read kp length: %w", err)
	}
	n := binary.BigEndian.Uint32(lb[:])
	if n == 0 || n > maxKPPayload {
		return nil, fmt.Errorf("invalid kp payload length %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(s, buf); err != nil {
		return nil, fmt.Errorf("read kp bytes: %w", err)
	}
	return buf, nil
}

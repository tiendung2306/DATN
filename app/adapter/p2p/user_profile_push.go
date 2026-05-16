package p2p

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// UserProfilePushProtocol carries a signed profile wire JSON + optional avatar bytes
// after both peers have completed /app/auth/1.0.0 (verified identity).
const UserProfilePushProtocol = protocol.ID("/app/user-profile/1.0.0")

const maxUserProfileWireBytes = 64 * 1024
const maxUserProfileSigBytes = 128
const maxUserProfileBlobBytes = 256 * 1024

// UserProfilePushHandler receives a verified remote peer's signed profile payload.
// Implementations must validate wire/signature against remotePeer before persisting.
type UserProfilePushHandler func(remotePeer peer.ID, wireJSON, signature, avatarBlob []byte) error

func readFramed(r io.Reader, max int) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint32(lenBuf[:]))
	if n < 0 || n > max {
		return nil, fmt.Errorf("frame length %d out of range (max %d)", n, max)
	}
	if n == 0 {
		return nil, nil
	}
	out := make([]byte, n)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, err
	}
	return out, nil
}

func writeFrame(w io.Writer, payload []byte, max int) error {
	if len(payload) > max {
		return fmt.Errorf("frame length %d exceeds max %d", len(payload), max)
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}

// InstallUserProfilePushHandler registers the stream handler on the host.
func InstallUserProfilePushHandler(h host.Host, fn UserProfilePushHandler) {
	h.SetStreamHandler(UserProfilePushProtocol, func(s network.Stream) {
		defer s.Close()
		remote := s.Conn().RemotePeer()
		_ = s.SetDeadline(time.Now().Add(45 * time.Second))
		wire, err := readFramed(s, maxUserProfileWireBytes)
		if err != nil {
			slog.Debug("user-profile: read wire", "peer", remote, "err", err)
			return
		}
		sig, err := readFramed(s, maxUserProfileSigBytes)
		if err != nil {
			slog.Debug("user-profile: read sig", "peer", remote, "err", err)
			return
		}
		blob, err := readFramed(s, maxUserProfileBlobBytes)
		if err != nil {
			slog.Debug("user-profile: read blob", "peer", remote, "err", err)
			return
		}
		if fn == nil {
			return
		}
		if err := fn(remote, wire, sig, blob); err != nil {
			slog.Debug("user-profile: handler rejected", "peer", remote, "err", err)
		}
	})
}

// PushUserProfileToPeer sends wire + signature + optional avatar blob to one peer.
func PushUserProfileToPeer(ctx context.Context, h host.Host, to peer.ID, wireJSON, signature, avatarBlob []byte) error {
	if len(wireJSON) == 0 || len(signature) == 0 {
		return fmt.Errorf("wire and signature are required")
	}
	if len(wireJSON) > maxUserProfileWireBytes || len(signature) > maxUserProfileSigBytes {
		return fmt.Errorf("wire or signature too large")
	}
	if len(avatarBlob) > maxUserProfileBlobBytes {
		return fmt.Errorf("avatar blob too large")
	}
	s, err := h.NewStream(ctx, to, UserProfilePushProtocol)
	if err != nil {
		return err
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(45 * time.Second))
	if err := writeFrame(s, wireJSON, maxUserProfileWireBytes); err != nil {
		return fmt.Errorf("write wire: %w", err)
	}
	if err := writeFrame(s, signature, maxUserProfileSigBytes); err != nil {
		return fmt.Errorf("write sig: %w", err)
	}
	if err := writeFrame(s, avatarBlob, maxUserProfileBlobBytes); err != nil {
		return fmt.Errorf("write blob: %w", err)
	}
	return nil
}

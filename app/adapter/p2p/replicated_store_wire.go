package p2p

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// Replica store direct sync protocols (verified peers only).
const (
	ReplicaStorePushProtocol = protocol.ID("/app/replica-store/push/1.0.0")
	ReplicaStoreSyncProtocol = protocol.ID("/app/replica-store/sync/1.0.0")
)

const (
	maxReplicaMetaBytes = 4096
	maxReplicaWireBytes = 64 * 1024
	maxReplicaSigBytes  = 128
	maxReplicaBlobBytes = 256 * 1024
	maxReplicaReqBytes  = 64 * 1024
)

// ReplicaPushMetaV1 is the first frame on ReplicaStorePushProtocol.
type ReplicaPushMetaV1 struct {
	V         int    `json:"v"`
	Namespace string `json:"namespace"`
	RecordKey string `json:"record_key"`
}

// ReplicaPullRequestV1 requests newer replicated rows from a peer.
type ReplicaPullRequestV1 struct {
	V         int              `json:"v"`
	Namespace string           `json:"namespace"`
	Keys      []string         `json:"keys"`
	Cursors   map[string]int64 `json:"cursors,omitempty"`
}

// ReplicaPullRecordHeaderV1 prefixes one pulled record on the sync stream.
type ReplicaPullRecordHeaderV1 struct {
	V        int    `json:"v"`
	Key      string `json:"key"`
	Revision int64  `json:"revision"`
}

func replicaReadFrame(r io.Reader, max int) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint32(lenBuf[:]))
	if n < 0 || n > max {
		return nil, fmt.Errorf("replica frame length %d out of range (max %d)", n, max)
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

func replicaWriteFrame(w io.Writer, payload []byte, max int) error {
	if len(payload) > max {
		return fmt.Errorf("replica frame length %d exceeds max %d", len(payload), max)
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

// ReplicaStorePushHandler receives a verified peer's replicated record push.
type ReplicaStorePushHandler func(remote peer.ID, meta ReplicaPushMetaV1, wireJSON, signature, blob []byte) error

// InstallReplicaStorePushHandler registers /app/replica-store/push/1.0.0.
func InstallReplicaStorePushHandler(h host.Host, fn ReplicaStorePushHandler) {
	h.SetStreamHandler(ReplicaStorePushProtocol, func(s network.Stream) {
		defer s.Close()
		remote := s.Conn().RemotePeer()
		_ = s.SetDeadline(time.Now().Add(45 * time.Second))
		metaBytes, err := replicaReadFrame(s, maxReplicaMetaBytes)
		if err != nil {
			slog.Debug("replica-store push: read meta", "peer", remote, "err", err)
			return
		}
		if len(metaBytes) == 0 {
			return
		}
		var meta ReplicaPushMetaV1
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			slog.Debug("replica-store push: meta json", "peer", remote, "err", err)
			return
		}
		wire, err := replicaReadFrame(s, maxReplicaWireBytes)
		if err != nil {
			slog.Debug("replica-store push: read wire", "peer", remote, "err", err)
			return
		}
		sig, err := replicaReadFrame(s, maxReplicaSigBytes)
		if err != nil {
			slog.Debug("replica-store push: read sig", "peer", remote, "err", err)
			return
		}
		blob, err := replicaReadFrame(s, maxReplicaBlobBytes)
		if err != nil {
			slog.Debug("replica-store push: read blob", "peer", remote, "err", err)
			return
		}
		if fn == nil {
			return
		}
		if err := fn(remote, meta, wire, sig, blob); err != nil {
			slog.Debug("replica-store push: rejected", "peer", remote, "err", err)
		}
	})
}

// PushReplicaStoreRecord sends meta + wire + signature + optional blob.
func PushReplicaStoreRecord(ctx context.Context, h host.Host, to peer.ID, meta ReplicaPushMetaV1, wireJSON, signature, blob []byte) error {
	if len(wireJSON) == 0 || len(signature) == 0 {
		return fmt.Errorf("wire and signature are required")
	}
	meta.V = 1
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if len(metaBytes) > maxReplicaMetaBytes {
		return fmt.Errorf("meta too large")
	}
	if len(wireJSON) > maxReplicaWireBytes || len(signature) > maxReplicaSigBytes {
		return fmt.Errorf("wire or signature too large")
	}
	if len(blob) > maxReplicaBlobBytes {
		return fmt.Errorf("blob too large")
	}
	s, err := h.NewStream(ctx, to, ReplicaStorePushProtocol)
	if err != nil {
		return err
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(45 * time.Second))
	if err := replicaWriteFrame(s, metaBytes, maxReplicaMetaBytes); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}
	if err := replicaWriteFrame(s, wireJSON, maxReplicaWireBytes); err != nil {
		return fmt.Errorf("write wire: %w", err)
	}
	if err := replicaWriteFrame(s, signature, maxReplicaSigBytes); err != nil {
		return fmt.Errorf("write sig: %w", err)
	}
	if err := replicaWriteFrame(s, blob, maxReplicaBlobBytes); err != nil {
		return fmt.Errorf("write blob: %w", err)
	}
	return nil
}

// ReplicaStoreSyncEmitFunc streams one record back to the puller.
type ReplicaStoreSyncEmitFunc func(hdr ReplicaPullRecordHeaderV1, wireJSON, signature, blob []byte) error

// ReplicaStoreSyncServer resolves pull requests against local SQLite.
type ReplicaStoreSyncServer func(remote peer.ID, req *ReplicaPullRequestV1, emit ReplicaStoreSyncEmitFunc) error

// InstallReplicaStoreSyncHandler registers /app/replica-store/sync/1.0.0.
func InstallReplicaStoreSyncHandler(h host.Host, fn ReplicaStoreSyncServer) {
	h.SetStreamHandler(ReplicaStoreSyncProtocol, func(s network.Stream) {
		defer s.Close()
		remote := s.Conn().RemotePeer()
		_ = s.SetDeadline(time.Now().Add(60 * time.Second))
		reqBytes, err := replicaReadFrame(s, maxReplicaReqBytes)
		if err != nil || len(reqBytes) == 0 {
			slog.Debug("replica-store sync: read req", "peer", remote, "err", err)
			return
		}
		var req ReplicaPullRequestV1
		if err := json.Unmarshal(reqBytes, &req); err != nil {
			slog.Debug("replica-store sync: req json", "peer", remote, "err", err)
			return
		}
		if fn == nil {
			return
		}
		emit := func(hdr ReplicaPullRecordHeaderV1, wireJSON, signature, blob []byte) error {
			hdr.V = 1
			hd, err := json.Marshal(hdr)
			if err != nil {
				return err
			}
			if err := replicaWriteFrame(s, hd, maxReplicaMetaBytes); err != nil {
				return err
			}
			if err := replicaWriteFrame(s, wireJSON, maxReplicaWireBytes); err != nil {
				return err
			}
			if err := replicaWriteFrame(s, signature, maxReplicaSigBytes); err != nil {
				return err
			}
			return replicaWriteFrame(s, blob, maxReplicaBlobBytes)
		}
		if err := fn(remote, &req, emit); err != nil {
			slog.Debug("replica-store sync: serve failed", "peer", remote, "err", err)
			return
		}
		_ = replicaWriteFrame(s, nil, maxReplicaMetaBytes)
	})
}

// ReplicaStoreSyncOnRecord is invoked for each record returned by a peer.
type ReplicaStoreSyncOnRecord func(key string, revision int64, wireJSON, signature, blob []byte) error

// PullReplicaStoreRecords requests profile/replicated rows from a remote peer.
func PullReplicaStoreRecords(ctx context.Context, h host.Host, remote peer.ID, req *ReplicaPullRequestV1, onRecord ReplicaStoreSyncOnRecord) error {
	req.V = 1
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if len(payload) > maxReplicaReqBytes {
		return fmt.Errorf("replica pull request too large")
	}
	s, err := h.NewStream(ctx, remote, ReplicaStoreSyncProtocol)
	if err != nil {
		return err
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(60 * time.Second))
	if err := replicaWriteFrame(s, payload, maxReplicaReqBytes); err != nil {
		return fmt.Errorf("write req: %w", err)
	}
	for {
		hdrBytes, err := replicaReadFrame(s, maxReplicaMetaBytes)
		if err != nil {
			return err
		}
		if len(hdrBytes) == 0 {
			break
		}
		var hdr ReplicaPullRecordHeaderV1
		if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
			return fmt.Errorf("header: %w", err)
		}
		wire, err := replicaReadFrame(s, maxReplicaWireBytes)
		if err != nil {
			return err
		}
		sig, err := replicaReadFrame(s, maxReplicaSigBytes)
		if err != nil {
			return err
		}
		blob, err := replicaReadFrame(s, maxReplicaBlobBytes)
		if err != nil {
			return err
		}
		if onRecord != nil {
			if err := onRecord(hdr.Key, hdr.Revision, wire, sig, blob); err != nil {
				return err
			}
		}
	}
	return nil
}

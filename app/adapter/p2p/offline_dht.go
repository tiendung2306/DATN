package p2p

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Per-sender inbox key: only the sender writes here (avoids DHT last-write-wins).
func offlineInboxDHTKey(recipient, groupID string, sender peer.ID) string {
	return "/app/inbox/" + recipient + "/" + groupID + "/" + sender.String()
}

type offlineInboxPayloadV1 struct {
	V         int       `json:"v"`
	Seqs      []int64   `json:"seqs"`
	Envelopes [][]byte `json:"envelopes"`
}

const dhtRecordLimit = 256 * 1024 // 256 KiB — Kademlia record size limit

// compressInboxPayload serialises and zlib-compresses an offlineInboxPayloadV1.
func compressInboxPayload(p offlineInboxPayloadV1) ([]byte, error) {
	plain, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(plain); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// StoreOfflineInboxBundle compresses and publishes one sender's pending ciphertext
// envelopes for a recipient into the DHT.
//
// If the compressed payload exceeds the DHT record limit (256 KiB) the oldest
// envelopes are dropped (from the front of the slice) until it fits, ensuring
// the most recent messages always reach the recipient.
func StoreOfflineInboxBundle(ctx context.Context, d *dht.IpfsDHT, recipient string, groupID string, sender peer.ID, seqs []int64, envelopes [][]byte) error {
	if len(seqs) != len(envelopes) || len(seqs) == 0 {
		return nil
	}

	// Trim oldest entries until the compressed payload fits within dhtRecordLimit.
	for len(seqs) > 0 {
		p := offlineInboxPayloadV1{V: 1, Seqs: seqs, Envelopes: envelopes}
		compressed, err := compressInboxPayload(p)
		if err != nil {
			return err
		}
		if len(compressed) <= dhtRecordLimit {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			key := offlineInboxDHTKey(recipient, groupID, sender)
			if err := d.PutValue(ctx, key, compressed); err != nil {
				return fmt.Errorf("DHT StoreOfflineInboxBundle %q: %w", key, err)
			}
			return nil
		}
		// Drop the oldest envelope and retry.
		seqs = seqs[1:]
		envelopes = envelopes[1:]
	}
	return fmt.Errorf("all envelopes individually exceed DHT record size limit")
}

// FetchOfflineInboxBundle retrieves and decodes a per-sender inbox for (recipient, group, sender).
func FetchOfflineInboxBundle(ctx context.Context, d *dht.IpfsDHT, recipient, groupID string, sender peer.ID) (seqs []int64, envelopes [][]byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	key := offlineInboxDHTKey(recipient, groupID, sender)
	raw, err := d.GetValue(ctx, key)
	if err != nil {
		return nil, nil, fmt.Errorf("DHT FetchOfflineInboxBundle %q: %w", key, err)
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("zlib: %w", err)
	}
	defer zr.Close()
	var plain bytes.Buffer
	if _, err := plain.ReadFrom(zr); err != nil {
		return nil, nil, err
	}
	var p offlineInboxPayloadV1
	if err := json.Unmarshal(plain.Bytes(), &p); err != nil {
		return nil, nil, err
	}
	if p.V != 1 || len(p.Seqs) != len(p.Envelopes) {
		return nil, nil, fmt.Errorf("invalid offline inbox payload")
	}
	return p.Seqs, p.Envelopes, nil
}

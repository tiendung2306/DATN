package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/pkg/filetransfer"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// FileTransferPrepareDTO is returned after ciphertext chunks are staged on disk (sender).
type FileTransferPrepareDTO struct {
	FileID             string `json:"file_id"`
	GroupID            string `json:"group_id"`
	PlaintextSHA256Hex string `json:"plaintext_sha256_hex"`
	PlaintextSize      int64  `json:"plaintext_size"`
	ChunkSize          int    `json:"chunk_size"`
	ChunkCount         int    `json:"chunk_count"`
	ExportEpoch        uint64 `json:"export_epoch"`
}

func (r *Runtime) registerFileTransferHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.SetStreamHandler(p2p.FileTransferProtocol, func(s network.Stream) {
		go r.handleFileTransferStream(s)
	})
	slog.Info("File-transfer handler registered")
}

func (r *Runtime) removeFileTransferHandler() {
	if r.node == nil {
		return
	}
	r.node.Host.RemoveStreamHandler(p2p.FileTransferProtocol)
}

func randomFileID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// PrepareOutgoingFileTransfer hashes the file (streaming), derives MLS exporter secrets with
// context=file_sha256, encrypts fixed-size chunks to .local, and records metadata for pull.
func (r *Runtime) PrepareOutgoingFileTransfer(groupID string, sourcePath string) (FileTransferPrepareDTO, error) {
	var zero FileTransferPrepareDTO
	if err := r.ensureSessionActive(); err != nil {
		return zero, err
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return zero, fmt.Errorf("group_id is required")
	}
	sourcePath = filepath.Clean(sourcePath)
	fi, err := os.Stat(sourcePath)
	if err != nil {
		return zero, fmt.Errorf("stat source file: %w", err)
	}
	if fi.IsDir() {
		return zero, fmt.Errorf("source path is a directory")
	}
	size := fi.Size()
	if size == 0 {
		return zero, fmt.Errorf("empty file")
	}

	r.mu.RLock()
	coord := r.coordinators[groupID]
	mls := r.mlsEngine
	r.mu.RUnlock()
	if coord == nil {
		return zero, ErrGroupNotFound
	}
	if mls == nil {
		return zero, fmt.Errorf("crypto engine not available")
	}

	chunkBytes := r.cfg.FileTransferChunkBytes
	if chunkBytes <= 0 {
		chunkBytes = filetransfer.DefaultChunkBytes
	}
	chunks := filetransfer.ChunkCount(size, chunkBytes)
	if chunks <= 0 {
		return zero, fmt.Errorf("invalid chunking")
	}

	f, err := os.Open(sourcePath)
	if err != nil {
		return zero, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return zero, fmt.Errorf("hash file: %w", err)
	}
	fileSum := h.Sum(nil)

	ctx := r.appCtx()
	if ctx == nil {
		ctx = context.Background()
	}
	groupState := coord.GetGroupState()
	exportEpoch := coord.CurrentEpoch()
	rawSecret, err := mls.ExportSecret(ctx, groupState, filetransfer.Label, fileSum, filetransfer.ExportSecretLen)
	if err != nil {
		return zero, fmt.Errorf("ExportSecret: %w", err)
	}
	aesKey, baseNonce, err := filetransfer.SplitExporterMaterial(rawSecret)
	if err != nil {
		return zero, err
	}

	fileID, err := randomFileID()
	if err != nil {
		return zero, err
	}
	outDir := filepath.Join(".local", "file-transfer", "out", fileID)
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return zero, fmt.Errorf("mkdir staging: %w", err)
	}

	f2, err := os.Open(sourcePath)
	if err != nil {
		return zero, err
	}
	defer f2.Close()

	remaining := size
	for i := 0; i < chunks; i++ {
		n := chunkBytes
		if int64(n) > remaining {
			n = int(remaining)
		}
		if n <= 0 {
			break
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(f2, buf); err != nil {
			return zero, fmt.Errorf("read plaintext chunk %d: %w", i, err)
		}
		ct, err := filetransfer.EncryptChunk(aesKey, baseNonce, uint64(i), buf)
		if err != nil {
			return zero, fmt.Errorf("encrypt chunk %d: %w", i, err)
		}
		chunkPath := filepath.Join(outDir, fmt.Sprintf("%08d.chunk", i))
		if err := os.WriteFile(chunkPath, ct, 0o600); err != nil {
			return zero, fmt.Errorf("write ciphertext chunk: %w", err)
		}
		remaining -= int64(n)
	}

	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	senderPeer := ""
	if node != nil {
		senderPeer = node.Host.ID().String()
	}

	rec := &store.FileTransferRecord{
		FileID:          fileID,
		GroupID:         groupID,
		Direction:       store.FileTransferDirectionOut,
		PlaintextSHA256: append([]byte(nil), fileSum...),
		PlaintextSize:   size,
		ChunkSize:       chunkBytes,
		ChunkCount:      chunks,
		ExportEpoch:     exportEpoch,
		SenderPeerID:    senderPeer,
		CiphertextDir:   outDir,
		State:           store.FileTransferStateReady,
	}
	if err := r.db.UpsertFileTransfer(rec); err != nil {
		return zero, err
	}

	r.emit("file:prepare", map[string]interface{}{
		"group_id": groupID,
		"file_id":  fileID,
		"bytes":    size,
	})

	return FileTransferPrepareDTO{
		FileID:             fileID,
		GroupID:            groupID,
		PlaintextSHA256Hex: hex.EncodeToString(fileSum),
		PlaintextSize:      size,
		ChunkSize:          chunkBytes,
		ChunkCount:         chunks,
		ExportEpoch:        exportEpoch,
	}, nil
}

func (r *Runtime) handleFileTransferStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(60 * time.Second))
	remote := s.Conn().RemotePeer()

	r.mu.RLock()
	node := r.node
	r.mu.RUnlock()
	if node == nil {
		return
	}
	ap := node.AuthProtocol
	if ap != nil && !ap.IsVerified(remote) {
		slog.Warn("file-transfer: unverified peer", "peer", remote)
		return
	}

	var req p2p.FilePullRequestV1
	if err := p2p.ReadFileTransferJSONFrame(s, &req); err != nil {
		slog.Warn("file-transfer: bad pull frame", "from", remote, "err", err)
		return
	}
	if req.V != 1 || strings.TrimSpace(req.FileID) == "" {
		slog.Warn("file-transfer: invalid pull request", "from", remote)
		return
	}

	rec, err := r.db.GetFileTransfer(strings.TrimSpace(req.FileID))
	if err != nil {
		if errors.Is(err, store.ErrFileTransferNotFound) {
			slog.Warn("file-transfer: unknown file_id", "file_id", req.FileID)
		} else {
			slog.Warn("file-transfer: db", "err", err)
		}
		return
	}
	if rec.Direction != store.FileTransferDirectionOut || rec.State != store.FileTransferStateReady {
		slog.Warn("file-transfer: not available", "file_id", req.FileID, "state", rec.State)
		return
	}

	r.mu.RLock()
	coord := r.coordinators[rec.GroupID]
	r.mu.RUnlock()
	if coord == nil || coord.CurrentEpoch() != rec.ExportEpoch {
		var cur uint64
		if coord != nil {
			cur = coord.CurrentEpoch()
		}
		slog.Warn("file-transfer: epoch mismatch or no coordinator",
			"file_id", rec.FileID, "current_epoch", cur, "need_epoch", rec.ExportEpoch)
		return
	}

	_ = s.SetDeadline(time.Now().Add(2 * time.Hour))

	manifest := p2p.FileManifestV1{
		V:                  1,
		FileID:             rec.FileID,
		GroupID:            rec.GroupID,
		PlaintextSHA256Hex: hex.EncodeToString(rec.PlaintextSHA256),
		PlaintextSize:      rec.PlaintextSize,
		ChunkSize:          rec.ChunkSize,
		ChunkCount:         rec.ChunkCount,
		ExportEpoch:        rec.ExportEpoch,
		SenderPeerID:       rec.SenderPeerID,
	}
	if err := p2p.WriteFileTransferJSONFrame(s, &manifest); err != nil {
		slog.Warn("file-transfer: write manifest", "err", err)
		return
	}

	for i := 0; i < rec.ChunkCount; i++ {
		p := filepath.Join(rec.CiphertextDir, fmt.Sprintf("%08d.chunk", i))
		ct, err := os.ReadFile(p)
		if err != nil {
			slog.Warn("file-transfer: read chunk", "path", p, "err", err)
			return
		}
		if err := p2p.WriteChunkPayload(s, uint64(i), ct); err != nil {
			slog.Warn("file-transfer: write chunk", "err", err)
			return
		}
	}

	r.emit("file:sent", map[string]interface{}{
		"group_id": rec.GroupID,
		"file_id":  rec.FileID,
		"peer":     remote.String(),
	})
}

// PullFileTransferFromPeer pulls ciphertext from the sender over /app/file/1.0.0 and writes decrypted plaintext to destPath.
func (r *Runtime) PullFileTransferFromPeer(groupID, fileID, senderPeerID, destPath string) error {
	if err := r.ensureSessionActive(); err != nil {
		return err
	}
	groupID = strings.TrimSpace(groupID)
	fileID = strings.TrimSpace(fileID)
	senderPeerID = strings.TrimSpace(senderPeerID)
	if groupID == "" || fileID == "" || senderPeerID == "" {
		return fmt.Errorf("group_id, file_id, and sender_peer_id are required")
	}
	destPath = filepath.Clean(destPath)
	if destPath == "" || destPath == "." {
		return fmt.Errorf("dest_path is required")
	}

	r.mu.RLock()
	coord := r.coordinators[groupID]
	mls := r.mlsEngine
	node := r.node
	r.mu.RUnlock()
	if coord == nil {
		return ErrGroupNotFound
	}
	if mls == nil {
		return fmt.Errorf("crypto engine not available")
	}
	if node == nil {
		return fmt.Errorf("P2P node not running")
	}

	remote, err := peer.Decode(senderPeerID)
	if err != nil {
		return fmt.Errorf("sender peer id: %w", err)
	}

	ctx := r.appCtx()
	if ctx == nil {
		ctx = context.Background()
	}

	if node.AuthProtocol != nil {
		node.AuthProtocol.InitiateHandshake(ctx, remote)
	}

	s, err := node.Host.NewStream(ctx, remote, p2p.FileTransferProtocol)
	if err != nil {
		return fmt.Errorf("open file-transfer stream: %w", err)
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(2 * time.Hour))

	pull := p2p.FilePullRequestV1{V: 1, FileID: fileID}
	if err := p2p.WriteFileTransferJSONFrame(s, &pull); err != nil {
		return fmt.Errorf("write pull: %w", err)
	}

	var manifest p2p.FileManifestV1
	if err := p2p.ReadFileTransferJSONFrame(s, &manifest); err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	if manifest.V != 1 {
		return fmt.Errorf("unsupported manifest version %d", manifest.V)
	}
	if strings.TrimSpace(manifest.GroupID) != groupID {
		return fmt.Errorf("manifest group mismatch: got %q want %q", manifest.GroupID, groupID)
	}
	if strings.TrimSpace(manifest.FileID) != fileID {
		return fmt.Errorf("manifest file_id mismatch")
	}

	localEpoch := coord.CurrentEpoch()
	if manifest.ExportEpoch != localEpoch {
		return fmt.Errorf("epoch mismatch: local=%d manifest=%d (wait for MLS group sync)", localEpoch, manifest.ExportEpoch)
	}

	sum, err := hex.DecodeString(strings.TrimSpace(manifest.PlaintextSHA256Hex))
	if err != nil || len(sum) != sha256.Size {
		return fmt.Errorf("invalid manifest plaintext_sha256_hex")
	}

	rawSecret, err := mls.ExportSecret(ctx, coord.GetGroupState(), filetransfer.Label, sum, filetransfer.ExportSecretLen)
	if err != nil {
		return fmt.Errorf("ExportSecret: %w", err)
	}
	aesKey, baseNonce, err := filetransfer.SplitExporterMaterial(rawSecret)
	if err != nil {
		return err
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer out.Close()

	h := sha256.New()
	var got int64

	for i := 0; i < manifest.ChunkCount; i++ {
		idx, ct, err := p2p.ReadChunkPayload(s)
		if err != nil {
			return fmt.Errorf("read chunk %d: %w", i, err)
		}
		if idx != uint64(i) {
			return fmt.Errorf("chunk index mismatch: want %d got %d", i, idx)
		}
		pt, err := filetransfer.DecryptChunk(aesKey, baseNonce, idx, ct)
		if err != nil {
			return fmt.Errorf("decrypt chunk %d: %w", i, err)
		}
		if _, err := out.Write(pt); err != nil {
			return err
		}
		if _, err := h.Write(pt); err != nil {
			return err
		}
		got += int64(len(pt))
	}

	if got != manifest.PlaintextSize {
		return fmt.Errorf("size mismatch: wrote %d manifest %d", got, manifest.PlaintextSize)
	}
	if subtle.ConstantTimeCompare(h.Sum(nil), sum) != 1 {
		return fmt.Errorf("plaintext sha256 mismatch after decrypt")
	}

	r.emit("file:received", map[string]interface{}{
		"group_id": groupID,
		"file_id":  fileID,
		"path":     destPath,
	})
	return nil
}

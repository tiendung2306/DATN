package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"app/adapter/p2p"
	"app/adapter/store"
	"app/pkg/filetransfer"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	// MaxFileTransferBytes caps MVP file transfer size to avoid long UI stalls.
	MaxFileTransferBytes = 100 * 1024 * 1024 // 100 MiB
)

var (
	// ErrUserCancelled indicates the user closed a native file dialog.
	ErrUserCancelled = errors.New("ERR_USER_CANCELLED")
)

// FileTransferPrepareDTO is returned after ciphertext chunks are staged on disk (sender).
type FileTransferPrepareDTO struct {
	FileID             string `json:"file_id"`
	GroupID            string `json:"group_id"`
	FileName           string `json:"file_name"`
	FileExt            string `json:"file_ext"`
	MimeType           string `json:"mime_type"`
	SenderPeerID       string `json:"sender_peer_id"`
	PlaintextSHA256Hex string `json:"plaintext_sha256_hex"`
	PlaintextSize      int64  `json:"plaintext_size"`
	ChunkSize          int    `json:"chunk_size"`
	ChunkCount         int    `json:"chunk_count"`
	ExportEpoch        uint64 `json:"export_epoch"`
}

type fileAttachmentMetadata struct {
	Type         string `json:"type"`
	FileID       string `json:"file_id"`
	Name         string `json:"name"`
	Ext          string `json:"ext,omitempty"`
	MimeType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	SHA256Hex    string `json:"sha256"`
	ChunkCount   int    `json:"chunk_count"`
	ChunkSize    int    `json:"chunk_size"`
	ExportEpoch  uint64 `json:"export_epoch"`
	SenderPeerID string `json:"sender_peer_id"`
}

type fileMessagePayload struct {
	Type string `json:"type"`
	fileAttachmentMetadata
}

type filePostPayload struct {
	Type        string                   `json:"type"`
	Title       string                   `json:"title,omitempty"`
	Body        string                   `json:"body"`
	Attachments []fileAttachmentMetadata `json:"attachments,omitempty"`
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

func fileNameAndExt(path string) (name string, ext string) {
	base := strings.TrimSpace(filepath.Base(path))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "unknown-file", ""
	}
	e := strings.ToLower(strings.TrimPrefix(filepath.Ext(base), "."))
	return base, e
}

func detectMimeType(path string) string {
	ext := strings.TrimSpace(strings.ToLower(filepath.Ext(path)))
	if ext != "" {
		if t := strings.TrimSpace(mime.TypeByExtension(ext)); t != "" {
			return t
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	var buf [512]byte
	n, err := f.Read(buf[:])
	if err != nil && !errors.Is(err, io.EOF) {
		return "application/octet-stream"
	}
	return http.DetectContentType(buf[:n])
}

func (r *Runtime) announcePreparedFile(groupID string, prepared FileTransferPrepareDTO) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("group_id is required")
	}
	if prepared.FileID == "" {
		return fmt.Errorf("missing file_id")
	}
	if prepared.SenderPeerID == "" {
		return fmt.Errorf("missing sender_peer_id")
	}
	if r.coordStorage == nil {
		return fmt.Errorf("group metadata storage not initialized")
	}
	rec, err := r.coordStorage.GetGroupRecord(groupID)
	if err != nil {
		return fmt.Errorf("group metadata unavailable: %w", err)
	}

	attach := fileAttachmentMetadata{
		Type:         "file",
		FileID:       prepared.FileID,
		Name:         prepared.FileName,
		Ext:          prepared.FileExt,
		MimeType:     prepared.MimeType,
		Size:         prepared.PlaintextSize,
		SHA256Hex:    prepared.PlaintextSHA256Hex,
		ChunkCount:   prepared.ChunkCount,
		ChunkSize:    prepared.ChunkSize,
		ExportEpoch:  prepared.ExportEpoch,
		SenderPeerID: prepared.SenderPeerID,
	}

	var payload []byte
	groupType := strings.TrimSpace(strings.ToLower(rec.GroupType))
	if groupType == "channel" {
		post := filePostPayload{
			Type:        "post",
			Title:       prepared.FileName,
			Body:        fmt.Sprintf("Da chia se tep %s (%d bytes).", prepared.FileName, prepared.PlaintextSize),
			Attachments: []fileAttachmentMetadata{attach},
		}
		payload, err = json.Marshal(post)
	} else {
		msg := fileMessagePayload{Type: "file", fileAttachmentMetadata: attach}
		payload, err = json.Marshal(msg)
	}
	if err != nil {
		return fmt.Errorf("marshal file announcement: %w", err)
	}
	return r.SendGroupMessage(groupID, string(payload))
}

// SendGroupFile opens a native picker, prepares encrypted chunks, and announces metadata to the group timeline.
func (r *Runtime) SendGroupFile(groupID string) (FileTransferPrepareDTO, error) {
	var zero FileTransferPrepareDTO
	if err := r.ensureSessionActive(); err != nil {
		return zero, err
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return zero, fmt.Errorf("group_id is required")
	}

	path, err := wailsRuntime.OpenFileDialog(r.appCtx(), wailsRuntime.OpenDialogOptions{
		Title: "Chon tep de gui",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return zero, fmt.Errorf("open dialog: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return zero, ErrUserCancelled
	}

	fi, err := os.Stat(path)
	if err != nil {
		return zero, fmt.Errorf("stat source file: %w", err)
	}
	if fi.IsDir() {
		return zero, fmt.Errorf("source path is a directory")
	}
	if fi.Size() > MaxFileTransferBytes {
		return zero, fmt.Errorf("ERR_FILE_TOO_LARGE: file exceeds %d bytes", MaxFileTransferBytes)
	}

	prepared, err := r.PrepareOutgoingFileTransfer(groupID, path)
	if err != nil {
		return zero, err
	}
	if err := r.announcePreparedFile(groupID, prepared); err != nil {
		return zero, err
	}
	return prepared, nil
}

// PrepareGroupFile opens a native picker and prepares encrypted chunks without announcing.
// Intended for composing posts/messages that include multiple attachments.
func (r *Runtime) PrepareGroupFile(groupID string) (FileTransferPrepareDTO, error) {
	var zero FileTransferPrepareDTO
	if err := r.ensureSessionActive(); err != nil {
		return zero, err
	}
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return zero, fmt.Errorf("group_id is required")
	}

	path, err := wailsRuntime.OpenFileDialog(r.appCtx(), wailsRuntime.OpenDialogOptions{
		Title: "Chon tep de dinh kem",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return zero, fmt.Errorf("open dialog: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return zero, ErrUserCancelled
	}
	fi, err := os.Stat(path)
	if err != nil {
		return zero, fmt.Errorf("stat source file: %w", err)
	}
	if fi.IsDir() {
		return zero, fmt.Errorf("source path is a directory")
	}
	if fi.Size() > MaxFileTransferBytes {
		return zero, fmt.Errorf("ERR_FILE_TOO_LARGE: file exceeds %d bytes", MaxFileTransferBytes)
	}
	return r.PrepareOutgoingFileTransfer(groupID, path)
}

// DownloadGroupFile opens a native save dialog and pulls/decrypts a file from sender.
func (r *Runtime) DownloadGroupFile(groupID, fileID, senderPeerID, suggestedName string) (string, error) {
	if err := r.ensureSessionActive(); err != nil {
		return "", err
	}
	groupID = strings.TrimSpace(groupID)
	fileID = strings.TrimSpace(fileID)
	senderPeerID = strings.TrimSpace(senderPeerID)
	if groupID == "" || fileID == "" || senderPeerID == "" {
		return "", fmt.Errorf("group_id, file_id, and sender_peer_id are required")
	}
	name := strings.TrimSpace(suggestedName)
	if name == "" {
		name = "downloaded-file"
	}

	destPath, err := wailsRuntime.SaveFileDialog(r.appCtx(), wailsRuntime.SaveDialogOptions{
		Title:           "Luu tep da giai ma",
		DefaultFilename: name,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("save dialog: %w", err)
	}
	if strings.TrimSpace(destPath) == "" {
		return "", ErrUserCancelled
	}
	if err := r.PullFileTransferFromPeer(groupID, fileID, senderPeerID, destPath); err != nil {
		return "", err
	}
	return destPath, nil
}

func openPathInOS(path string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/c", "start", "", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

// OpenDownloadedFile opens a previously downloaded file by file_id.
// It first resolves persisted path from DB (works across app restarts), then
// falls back to a caller-provided path if present.
func (r *Runtime) OpenDownloadedFile(groupID, fileID, fallbackPath string) (string, error) {
	if err := r.ensureSessionActive(); err != nil {
		return "", err
	}
	groupID = strings.TrimSpace(groupID)
	fileID = strings.TrimSpace(fileID)
	if groupID == "" || fileID == "" {
		return "", fmt.Errorf("group_id and file_id are required")
	}

	var chosen string
	if r.db != nil {
		if rec, err := r.db.GetFileTransfer(fileID); err == nil {
			if rec.Direction == store.FileTransferDirectionIn && rec.GroupID == groupID {
				chosen = strings.TrimSpace(rec.CiphertextDir)
			}
		}
	}
	if chosen == "" {
		chosen = strings.TrimSpace(fallbackPath)
	}
	if chosen == "" {
		return "", fmt.Errorf("ERR_FILE_NOT_DOWNLOADED: file has not been downloaded on this device")
	}
	if _, err := os.Stat(chosen); err != nil {
		return "", fmt.Errorf("ERR_FILE_MISSING_LOCAL: %w", err)
	}
	if err := openPathInOS(chosen); err != nil {
		return "", fmt.Errorf("ERR_FILE_OPEN_FAILED: %w", err)
	}
	return chosen, nil
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
	if size > MaxFileTransferBytes {
		return zero, fmt.Errorf("ERR_FILE_TOO_LARGE: file exceeds %d bytes", MaxFileTransferBytes)
	}
	fileName, fileExt := fileNameAndExt(sourcePath)
	mimeType := detectMimeType(sourcePath)

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
		FileName:           fileName,
		FileExt:            fileExt,
		MimeType:           mimeType,
		SenderPeerID:       senderPeer,
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
	inRec := &store.FileTransferRecord{
		FileID:          fileID,
		GroupID:         groupID,
		Direction:       store.FileTransferDirectionIn,
		PlaintextSHA256: append([]byte(nil), sum...),
		PlaintextSize:   manifest.PlaintextSize,
		ChunkSize:       manifest.ChunkSize,
		ChunkCount:      manifest.ChunkCount,
		ExportEpoch:     manifest.ExportEpoch,
		SenderPeerID:    senderPeerID,
		CiphertextDir:   destPath,
		State:           store.FileTransferStateTransferring,
	}
	if r.db != nil {
		_ = r.db.UpsertFileTransfer(inRec)
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
			if r.db != nil {
				inRec.State = store.FileTransferStateFailed
				_ = r.db.UpsertFileTransfer(inRec)
			}
			return fmt.Errorf("read chunk %d: %w", i, err)
		}
		if idx != uint64(i) {
			if r.db != nil {
				inRec.State = store.FileTransferStateFailed
				_ = r.db.UpsertFileTransfer(inRec)
			}
			return fmt.Errorf("chunk index mismatch: want %d got %d", i, idx)
		}
		pt, err := filetransfer.DecryptChunk(aesKey, baseNonce, idx, ct)
		if err != nil {
			if r.db != nil {
				inRec.State = store.FileTransferStateFailed
				_ = r.db.UpsertFileTransfer(inRec)
			}
			return fmt.Errorf("decrypt chunk %d: %w", i, err)
		}
		if _, err := out.Write(pt); err != nil {
			if r.db != nil {
				inRec.State = store.FileTransferStateFailed
				_ = r.db.UpsertFileTransfer(inRec)
			}
			return err
		}
		if _, err := h.Write(pt); err != nil {
			if r.db != nil {
				inRec.State = store.FileTransferStateFailed
				_ = r.db.UpsertFileTransfer(inRec)
			}
			return err
		}
		got += int64(len(pt))
	}

	if got != manifest.PlaintextSize {
		if r.db != nil {
			inRec.State = store.FileTransferStateFailed
			_ = r.db.UpsertFileTransfer(inRec)
		}
		return fmt.Errorf("size mismatch: wrote %d manifest %d", got, manifest.PlaintextSize)
	}
	if subtle.ConstantTimeCompare(h.Sum(nil), sum) != 1 {
		if r.db != nil {
			inRec.State = store.FileTransferStateFailed
			_ = r.db.UpsertFileTransfer(inRec)
		}
		return fmt.Errorf("plaintext sha256 mismatch after decrypt")
	}
	if r.db != nil {
		inRec.State = store.FileTransferStateCompleted
		_ = r.db.UpsertFileTransfer(inRec)
	}

	r.emit("file:received", map[string]interface{}{
		"group_id": groupID,
		"file_id":  fileID,
		"path":     destPath,
	})
	return nil
}

package p2p

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/protocol"
)

// FileTransferProtocol is the MVP direct sender→receiver chunked file transport.
const FileTransferProtocol = protocol.ID("/app/file/1.0.0")

const fileTransferMaxJSONFrame = 4 << 20 // 4 MiB

// FilePullRequestV1 is sent by the receiver (puller) as the first JSON frame.
type FilePullRequestV1 struct {
	V      int    `json:"v"`
	FileID string `json:"file_id"`
}

// FileManifestV1 is sent by the sender immediately after accepting a pull.
type FileManifestV1 struct {
	V                  int    `json:"v"`
	FileID             string `json:"file_id"`
	GroupID            string `json:"group_id"`
	PlaintextSHA256Hex string `json:"plaintext_sha256_hex"`
	PlaintextSize      int64  `json:"plaintext_size"`
	ChunkSize          int    `json:"chunk_size"`
	ChunkCount         int    `json:"chunk_count"`
	ExportEpoch        uint64 `json:"export_epoch"`
	SenderPeerID       string `json:"sender_peer_id"`
}

func WriteFileTransferJSONFrame(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > fileTransferMaxJSONFrame {
		return fmt.Errorf("file-transfer json frame size %d invalid", len(data))
	}
	var lb [4]byte
	binary.BigEndian.PutUint32(lb[:], uint32(len(data)))
	if _, err := w.Write(lb[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func ReadFileTransferJSONFrame(r io.Reader, out interface{}) error {
	var lb [4]byte
	if _, err := io.ReadFull(r, lb[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(lb[:])
	if n == 0 || n > fileTransferMaxJSONFrame {
		return fmt.Errorf("file-transfer json frame size %d invalid", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, out)
}

// Chunk wire format: [8B chunk_index BE][4B ciphertext_len BE][ciphertext]
const chunkHeaderLen = 12

// WriteChunkPayload writes one ciphertext chunk with index.
func WriteChunkPayload(w io.Writer, chunkIndex uint64, ciphertext []byte) error {
	if len(ciphertext) > fileTransferMaxJSONFrame {
		return fmt.Errorf("chunk too large: %d", len(ciphertext))
	}
	var hdr [chunkHeaderLen]byte
	binary.BigEndian.PutUint64(hdr[0:8], chunkIndex)
	binary.BigEndian.PutUint32(hdr[8:12], uint32(len(ciphertext)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(ciphertext)
	return err
}

// ReadChunkPayload reads one chunk or returns io.EOF if no header could be read at clean close.
func ReadChunkPayload(r io.Reader) (chunkIndex uint64, ciphertext []byte, err error) {
	var hdr [chunkHeaderLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	chunkIndex = binary.BigEndian.Uint64(hdr[0:8])
	n := binary.BigEndian.Uint32(hdr[8:12])
	if n > fileTransferMaxJSONFrame {
		return 0, nil, fmt.Errorf("chunk payload length %d invalid", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, nil, err
	}
	return chunkIndex, buf, nil
}

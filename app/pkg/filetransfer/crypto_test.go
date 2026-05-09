package filetransfer

import (
	"bytes"
	"testing"
)

func TestNonceForChunkUnique(t *testing.T) {
	base := bytes.Repeat([]byte{0xAB}, 12)
	seen := map[string]struct{}{}
	for i := uint64(0); i < 1000; i++ {
		n, err := NonceForChunk(base, i)
		if err != nil {
			t.Fatal(err)
		}
		s := string(n)
		if _, ok := seen[s]; ok {
			t.Fatalf("duplicate nonce at index %d", i)
		}
		seen[s] = struct{}{}
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	sec := make([]byte, ExportSecretLen)
	for i := range sec {
		sec[i] = byte(i + 1)
	}
	key, baseNonce, err := SplitExporterMaterial(sec)
	if err != nil {
		t.Fatal(err)
	}
	pt := []byte("hello chunk world")
	ct, err := EncryptChunk(key, baseNonce, 7, pt)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecryptChunk(key, baseNonce, 7, ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, pt) {
		t.Fatalf("plaintext mismatch")
	}
}

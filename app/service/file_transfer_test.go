package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenFileTransferDestination_RemovesPartialFileOnFailure(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "partial.bin")

	out, cleanup, err := openFileTransferDestination(destPath)
	if err == nil {
		_, _ = out.Write([]byte("partial"))
		failErr := os.ErrInvalid
		completed := false
		cleanup(&failErr, &completed)
		err = failErr
	}
	if out == nil || cleanup == nil {
		t.Fatal("expected file and cleanup helper")
	}
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Fatalf("dest file still exists after failure: statErr=%v", statErr)
	}
}

func TestOpenFileTransferDestination_PreservesCompletedFile(t *testing.T) {
	t.Parallel()

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "complete.bin")

	out, cleanup, err := openFileTransferDestination(destPath)
	if err != nil {
		t.Fatalf("openFileTransferDestination: %v", err)
	}
	if _, err := out.Write([]byte("ok")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	completed := true
	cleanup(&err, &completed)
	if err != nil {
		t.Fatalf("cleanup completed file: %v", err)
	}
	if _, statErr := os.Stat(destPath); statErr != nil {
		t.Fatalf("completed file missing: %v", statErr)
	}
}

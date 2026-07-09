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

func TestSanitizeSuggestedFileName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"report.pdf", "report.pdf"},
		{"  bad:file<name>.txt  ", "bad_file_name_.txt"},
		{"path/to/file.png", "path_to_file.png"},
		{"windows\\name.docx", "windows_name.docx"},
		{"file\x00with\x1fctrl", "filewithctrl"},
		{"", "downloaded-file"},
		{"   ", "downloaded-file"},
		{"...", "downloaded-file"},
	}
	for _, c := range cases {
		got := sanitizeSuggestedFileName(c.in)
		if got != c.want {
			t.Errorf("sanitizeSuggestedFileName(%q) = %q, want %q", c.in, got, c.want)
		}
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

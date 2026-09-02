// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadAndCleanupBinaryTempFileNil(t *testing.T) {
	data, err := ReadAndCleanupBinaryTempFile(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Fatalf("expected nil data, got %v", data)
	}
}

func TestReadAndCleanupBinaryTempFileReadsAndCleansUpTempFile(t *testing.T) {
	want := []byte{0xde, 0xad, 0xbe, 0xef}

	tmpFile, err := os.CreateTemp("", "binaryfile-test")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	})
	if _, err = tmpFile.Write(want); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	path := tmpFile.Name()

	got, err := ReadAndCleanupBinaryTempFile(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected temp file %q to be removed, stat error: %v", path, err)
	}
}

func TestReadAndCleanupBinaryTempFileFallsBackWhenSeekFails(t *testing.T) {
	want := []byte{0x01, 0x02, 0x03}

	tmpFile, err := os.CreateTemp("", "binaryfile-test-fallback")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	path := tmpFile.Name()
	t.Cleanup(func() {
		_ = os.Remove(path)
	})
	if _, err = tmpFile.Write(want); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	// Close the file so Seek/ReadAll fail, forcing the os.ReadFile fallback path.
	if err = tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	got, err := ReadAndCleanupBinaryTempFile(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected temp file %q to be removed, stat error: %v", path, err)
	}
}

func TestCleanupBinaryTempFileNil(t *testing.T) {
	if err := CleanupBinaryTempFile(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCleanupBinaryTempFileRemovesTempFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "binaryfile-cleanup-test")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	path := tmpFile.Name()
	t.Cleanup(func() {
		_ = tmpFile.Close()
		_ = os.Remove(path)
	})

	if err = CleanupBinaryTempFile(tmpFile); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected temp file %q to be removed, stat error: %v", path, err)
	}
}

// A file already closed and removed by an earlier cleanup call is the common case for a
// pending message whose payload was captured at storage time: not an error to report.
func TestCleanupBinaryTempFileToleratesAlreadyRemoved(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "binaryfile-cleanup-test-twice")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	path := tmpFile.Name()
	t.Cleanup(func() {
		_ = tmpFile.Close()
		_ = os.Remove(path)
	})

	if err = CleanupBinaryTempFile(tmpFile); err != nil {
		t.Fatalf("first cleanup: unexpected error: %v", err)
	}

	if err = CleanupBinaryTempFile(tmpFile); err != nil {
		t.Fatalf("second cleanup on an already-removed file must not error, got: %v", err)
	}
}

func TestCleanupBinaryTempFileDoesNotRemoveNonTempFile(t *testing.T) {
	dir, err := os.MkdirTemp(".", "binaryfile-cleanup-test-non-temp")
	if err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	path := filepath.Join(dir, "not-a-temp-file")
	if err = os.WriteFile(path, []byte{0x01}, 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if err = CleanupBinaryTempFile(f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err = os.Stat(path); err != nil {
		t.Fatalf("expected non-temp file %q to still exist, stat error: %v", path, err)
	}
}

func TestReadAndCleanupBinaryTempFileDoesNotRemoveNonTempFile(t *testing.T) {
	want := []byte{0x01, 0x02, 0x03}

	// Use the current working directory rather than t.TempDir(), since the
	// latter is itself created under os.TempDir() and would defeat the test.
	dir, err := os.MkdirTemp(".", "binaryfile-test-non-temp")
	if err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	path := filepath.Join(dir, "not-a-temp-file")
	if err = os.WriteFile(path, want, 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	got, err := ReadAndCleanupBinaryTempFile(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	if _, err = os.Stat(path); err != nil {
		t.Fatalf("expected non-temp file %q to still exist, stat error: %v", path, err)
	}
}

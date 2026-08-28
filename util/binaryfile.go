// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadAndCleanupBinaryTempFile reads a binary multipart part backed by a temp file
// created by the openapi client, then closes and removes the temp file.
func ReadAndCleanupBinaryTempFile(file *os.File) ([]byte, error) {
	if file == nil {
		return nil, nil
	}
	defer func() {
		name := file.Name()
		_ = file.Close()
		if name != "" && isUnderTempDir(name) {
			_ = os.Remove(name)
		}
	}()
	_, seekErr := file.Seek(0, io.SeekStart)
	var readErr error
	if seekErr == nil {
		var data []byte
		data, readErr = io.ReadAll(file)
		if readErr == nil {
			return data, nil
		}
	}
	name := file.Name()
	if name == "" {
		return nil, fmt.Errorf("read binary temp file: seek error: %v, read error: %v", seekErr, readErr)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read binary temp file %q: %w (seek error: %v, read error: %v)",
			name, err, seekErr, readErr)
	}
	return data, nil
}

// isUnderTempDir reports whether name resides in os.TempDir(), guarding the
// cleanup in ReadAndCleanupBinaryTempFile from removing non-temp files.
func isUnderTempDir(name string) bool {
	tempDir := filepath.Clean(os.TempDir())
	dir := filepath.Clean(filepath.Dir(name))
	return dir == tempDir || strings.HasPrefix(dir, tempDir+string(os.PathSeparator))
}

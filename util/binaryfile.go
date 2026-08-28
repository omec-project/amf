// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"io"
	"os"
)

// ReadBinaryResponseFile reads a binary response part backed by a temp file
// created by the openapi client, then closes and removes the temp file.
func ReadBinaryResponseFile(file *os.File) ([]byte, error) {
	if file == nil {
		return nil, nil
	}
	defer func() {
		name := file.Name()
		file.Close()
		if name != "" {
			os.Remove(name)
		}
	}()
	if _, err := file.Seek(0, io.SeekStart); err == nil {
		data, readErr := io.ReadAll(file)
		if readErr == nil {
			return data, nil
		}
	}
	if file.Name() == "" {
		return nil, fmt.Errorf("unable to read binary response file")
	}
	return os.ReadFile(file.Name())
}

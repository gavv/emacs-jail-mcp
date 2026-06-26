// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package container

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed elisp/*.el
var elispFiles embed.FS

// WriteElispFiles writes embedded elisp files to dir, creating it if needed.
func WriteElispFiles(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create elisp dir %q: %w", dir, err)
	}

	entries, err := elispFiles.ReadDir("elisp")
	if err != nil {
		return fmt.Errorf("read embedded elisp dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		data, err := elispFiles.ReadFile("elisp/" + name)
		if err != nil {
			return fmt.Errorf("read embedded file %q: %w", name, err)
		}
		dst := filepath.Join(dir, name)
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write %q: %w", dst, err)
		}
	}

	return nil
}

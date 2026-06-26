// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package container_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gavv/emacs-jail-mcp/internal/container"
)

func TestWriteElispFilesCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "subdir", "elisp")

	require.NoError(t, container.WriteElispFiles(dir), "WriteElispFiles")

	info, err := os.Stat(dir)
	require.NoError(t, err, "dir not created")
	assert.True(t, info.IsDir(), "expected a directory at %q", dir)
}

func TestWriteElispFilesWritesFiles(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, container.WriteElispFiles(dir), "WriteElispFiles")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "ReadDir")
	require.NotEmpty(t, entries, "WriteElispFiles wrote no files")
	for _, e := range entries {
		assert.True(t, filepath.Ext(e.Name()) == ".el",
			"unexpected non-.el file %q in elisp dir", e.Name())
	}
}

func TestWriteElispFilesContainsJailRpc(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, container.WriteElispFiles(dir), "WriteElispFiles")

	target := filepath.Join(dir, "emacs-jail-rpc.el")
	info, err := os.Stat(target)
	require.NoError(t, err, "emacs-jail-rpc.el not written")
	assert.Greater(t, info.Size(), int64(0), "emacs-jail-rpc.el is empty")
}

func TestWriteElispFilesIdempotent(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, container.WriteElispFiles(dir), "first WriteElispFiles")
	require.NoError(t, container.WriteElispFiles(dir), "second WriteElispFiles")
}

func TestWriteElispFilesInvalidDir(t *testing.T) {
	// /proc/nonexistent is not creatable on Linux.
	err := container.WriteElispFiles("/proc/nonexistent/elisp")
	assert.Error(t, err)
}

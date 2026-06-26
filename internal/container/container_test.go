// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package container_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gavv/emacs-jail-mcp/internal/config"
	"github.com/gavv/emacs-jail-mcp/internal/container"
	"github.com/gavv/emacs-jail-mcp/internal/display"
)

func testDisplay() *display.Display {
	return display.Default(20)
}

func TestStopWhenNotRunning(t *testing.T) {
	cfg := config.DefaultServer()
	c := container.New(cfg, testDisplay())

	err := c.Stop(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestIsRunningInitiallyFalse(t *testing.T) {
	cfg := config.DefaultServer()
	c := container.New(cfg, testDisplay())
	assert.False(t, c.IsRunning())
}

func TestLogLinesEmptyWhenNoFile(t *testing.T) {
	cfg := config.DefaultServer()
	c := container.New(cfg, testDisplay())

	lines := c.LogLines(10)
	assert.Empty(t, lines)
}

func TestLogLinesAllLines(t *testing.T) {
	cfg := config.DefaultServer()
	cfg.EmacsSocketDir = t.TempDir()
	cfg.ContainerPrefix = "test-loglines-all"

	content := "line1\nline2\nline3\n"
	require.NoError(t, os.WriteFile(cfg.LogPath(), []byte(content), 0o644), "write log")
	t.Cleanup(func() { _ = os.Remove(cfg.LogPath()) })

	c := container.New(cfg, testDisplay())
	lines := c.LogLines(0)
	assert.Len(t, lines, 3, "LogLines(0) should return all 3 lines; lines=%v", lines)
}

func TestLogLinesLimited(t *testing.T) {
	cfg := config.DefaultServer()
	cfg.EmacsSocketDir = t.TempDir()
	cfg.ContainerPrefix = "test-loglines-limited"

	content := "a\nb\nc\nd\ne\n"
	require.NoError(t, os.WriteFile(cfg.LogPath(), []byte(content), 0o644), "write log")
	t.Cleanup(func() { _ = os.Remove(cfg.LogPath()) })

	c := container.New(cfg, testDisplay())
	lines := c.LogLines(2)
	require.Len(t, lines, 2, "LogLines(2) should return 2 lines; lines=%v", lines)
	assert.Equal(t, []string{"d", "e"}, lines)
}

func TestLogLinesNegative(t *testing.T) {
	cfg := config.DefaultServer()
	cfg.EmacsSocketDir = t.TempDir()
	cfg.ContainerPrefix = "test-loglines-neg"

	content := "x\ny\n"
	require.NoError(t, os.WriteFile(cfg.LogPath(), []byte(content), 0o644), "write log")
	t.Cleanup(func() { _ = os.Remove(cfg.LogPath()) })

	c := container.New(cfg, testDisplay())
	lines := c.LogLines(-1)
	assert.Len(t, lines, 2, "LogLines(-1) should return all 2 lines; lines=%v", lines)
}

// TestLogLinesRequestMoreThanExist verifies that n > line count returns all lines
// without panic.
func TestLogLinesRequestMoreThanExist(t *testing.T) {
	cfg := config.DefaultServer()
	cfg.EmacsSocketDir = t.TempDir()
	cfg.ContainerPrefix = "test-loglines-more"

	content := "only\n"
	require.NoError(t, os.WriteFile(cfg.LogPath(), []byte(content), 0o644), "write log")
	t.Cleanup(func() { _ = os.Remove(cfg.LogPath()) })

	c := container.New(cfg, testDisplay())
	lines := c.LogLines(100)
	assert.Len(t, lines, 1, "LogLines(100) on 1-line file should return 1 line; lines=%v", lines)
}

// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

//go:build e2e

package e2e_test

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gavv/emacs-jail-mcp/internal/cli"
)

// runCLI runs the root cobra command with the given args and returns captured stdout and
// any execution error.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err, "os.Pipe")
	os.Stdout = w

	cmd := cli.NewRootCmd()
	cmd.SetArgs(args)
	execErr := cmd.Execute()

	_ = w.Close()
	os.Stdout = origStdout

	out, readErr := io.ReadAll(r)
	_ = r.Close()
	require.NoError(t, readErr)

	return string(out), execErr
}

// TestCLI exercises CLI subcommands against a live jail. The server is started via
// startServer() (cobra command with --mcp-port 9422), and all client subcommands connect
// to it via --mcp-port.
func TestCLI(t *testing.T) {
	var srv TestServer

	t.Run("InfoBeforeStart", func(t *testing.T) {
		out, err := runCLI(t, "info", "--mcp-port", srv.Port())
		require.NoError(t, err, "info before start")
		assert.Contains(t, out, "Address:")
		assert.Contains(t, out, "offline")
	})

	srv.Start(t)
	t.Cleanup(func() { srv.Stop(t) })

	t.Run("InfoAfterStart", func(t *testing.T) {
		out, err := runCLI(t, "info", "--mcp-port", srv.Port())
		require.NoError(t, err, "info after start")
		assert.Contains(t, out, "Address:")
		assert.Contains(t, out, "online")
		assert.NotContains(t, out, "offline")
	})

	t.Run("Control/Start", func(t *testing.T) {
		out, err := runCLI(t,
			"send", "--mcp-port", srv.Port(),
			"control", "--start",
		)
		require.NoError(t, err, "control start")
		require.Contains(t, out, "started")

		out, err = runCLI(t,
			"send", "--mcp-port", srv.Port(),
			"control", "--status",
		)
		require.NoError(t, err, "control status")
		assert.Contains(t, out, "running")
	})

	t.Run("Eval", func(t *testing.T) {
		out, err := runCLI(t, "send", "--mcp-port", srv.Port(), "eval", "(+ 10 20)")
		require.NoError(t, err, "eval")
		assert.Contains(t, out, "30")
	})

	t.Run("Shell", func(t *testing.T) {
		out, err := runCLI(t,
			"send", "--mcp-port", srv.Port(), "shell", "echo cli-shell-ok")
		require.NoError(t, err, "shell")
		assert.Contains(t, out, "cli-shell-ok")
	})

	t.Run("Logs", func(t *testing.T) {
		out, err := runCLI(t,
			"send", "--mcp-port", srv.Port(), "logs", "--sources", "init_log")
		require.NoError(t, err, "logs")
		assert.Contains(t, out, "=== init_log ===")
	})

	t.Run("Bytecomp", func(t *testing.T) {
		elFile := "/tmp/cli-bytecomp-test.el"
		elContent := ";;; -*- lexical-binding: t -*-\n" +
			"(defun cli-test () t)\n"

		_, shellErr := runCLI(t,
			"send", "--mcp-port", srv.Port(),
			"shell", "printf '"+elContent+"' > "+elFile,
		)
		require.NoError(t, shellErr, "write el file")

		out, err := runCLI(t,
			"send", "--mcp-port", srv.Port(),
			"bytecomp", "--file-path", elFile,
		)
		require.NoError(t, err, "bytecomp")
		assert.Contains(t, out, `"file"`)
	})

	t.Run("Screenshot", func(t *testing.T) {
		outFile := t.TempDir() + "/cli-screenshot.png"
		_, err := runCLI(t,
			"send", "--mcp-port", srv.Port(), "screenshot", "--output", outFile)
		require.NoError(t, err, "screenshot")

		data, readErr := os.ReadFile(outFile)
		require.NoError(t, readErr, "read screenshot")
		require.GreaterOrEqual(t, len(data), 8)

		const screenshotPNGMagic = "\x89PNG"
		assert.Equal(t, screenshotPNGMagic, string(data[:4]))
	})

	t.Run("Control/Stop", func(t *testing.T) {
		out, err := runCLI(t,
			"send", "--mcp-port", srv.Port(),
			"control", "--stop",
		)
		require.NoError(t, err, "control stop")
		require.Contains(t, out, "stopped")

		out, err = runCLI(t,
			"send", "--mcp-port", srv.Port(),
			"control", "--status",
		)
		require.NoError(t, err, "control status after stop")
		assert.Contains(t, out, "stopped")
	})
}

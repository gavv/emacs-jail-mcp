// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

//go:build e2e

package e2e_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	e2eTimeout   = 240 * time.Second
	startTimeout = 240 * time.Second
	evalTimeout  = 30 * time.Second
)

// newMCPClient creates an SSE MCP client connected to the server via TCP and performs
// the MCP initialize handshake.
func newMCPClient(t *testing.T, srv *TestServer) *mcpclient.Client {
	t.Helper()

	baseURL := fmt.Sprintf("http://%s/sse", srv.Addr())

	c, err := mcpclient.NewSSEMCPClient(baseURL)
	require.NoError(t, err, "create SSE client")
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), e2eTimeout)
	t.Cleanup(cancel)

	require.NoError(t, c.Start(ctx), "SSE client start")

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "e2e-test",
		Version: "0.0.1",
	}
	_, err = c.Initialize(ctx, initReq)
	require.NoError(t, err, "initialize")

	return c
}

func callTool(
	t *testing.T,
	c *mcpclient.Client,
	name string,
	args map[string]any,
) *mcp.CallToolResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), evalTimeout)
	defer cancel()

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := c.CallTool(ctx, req)
	require.NoError(t, err, "CallTool %q", name)
	return result
}

func callToolLong(
	t *testing.T,
	c *mcpclient.Client,
	name string,
	args map[string]any,
	timeout time.Duration,
) *mcp.CallToolResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := c.CallTool(ctx, req)
	require.NoError(t, err, "CallTool %q", name)
	return result
}

func firstText(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func firstImageData(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if ic, ok := c.(mcp.ImageContent); ok {
			return ic.Data
		}
	}
	return ""
}

// TestMCP is the main end-to-end test suite.
func TestMCP(t *testing.T) {
	var srv TestServer

	srv.Start(t)
	t.Cleanup(func() { srv.Stop(t) })

	c := newMCPClient(t, &srv)

	t.Run("ListTools", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result, err := c.ListTools(ctx, mcp.ListToolsRequest{})
		require.NoError(t, err, "ListTools")

		want := []string{
			"control",
			"logs",
			"eval",
			"shell",
			"screenshot",
			"bytecomp",
		}
		assert.Len(t, result.Tools, len(want))
		nameSet := make(map[string]bool)
		for _, tool := range result.Tools {
			nameSet[tool.Name] = true
		}
		for _, name := range want {
			assert.True(t, nameSet[name], "tool %q not registered", name)
		}
	})

	t.Run("EvalBeforeStart", func(t *testing.T) {
		result := callTool(t, c, "eval",
			map[string]any{"expression": "(+ 1 2)"})
		assert.True(t, result.IsError,
			"expected error result, got success: %s",
			firstText(result))
		assert.Contains(t, firstText(result), "not running")
	})

	t.Run("Control/StatusBeforeStart", func(t *testing.T) {
		result := callTool(t, c, "control", map[string]any{"action": "status"})
		assert.False(t, result.IsError,
			"expected success, got error: %s",
			firstText(result))
		assert.Contains(t, firstText(result), "stopped")
	})

	t.Run("Control/Start", func(t *testing.T) {
		result := callToolLong(t, c, "control",
			map[string]any{"action": "start"},
			startTimeout,
		)
		require.False(t, result.IsError,
			"expected success, got error: %s", firstText(result))
		require.Contains(t, firstText(result), "started")
	})

	initLogsResult := callTool(t, c, "logs",
		map[string]any{"sources": "init_log"})
	initLogs := firstText(initLogsResult)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("Emacs init log:\n%s", initLogs)
		}
	})

	t.Run("Logs", func(t *testing.T) {
		t.Run("InitLog", func(t *testing.T) {
			result := callTool(t, c, "logs",
				map[string]any{"sources": "init_log"})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			text := firstText(result)
			assert.NotContains(t, text, "(empty)")
			assert.Contains(t, text, "=== init_log ===")
		})

		t.Run("Stderr", func(t *testing.T) {
			result := callTool(t, c, "logs", map[string]any{"sources": "stderr"})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			assert.Contains(t, firstText(result), "=== stderr ===")
		})

		t.Run("Buffers", func(t *testing.T) {
			marker := "e2e-logs-buffer-marker-99887"
			callTool(t, c, "eval",
				map[string]any{
					"expression": `(message "` +
						marker + `")`,
				})

			result := callTool(t, c, "logs",
				map[string]any{"sources": "messages"})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			assert.Contains(t, firstText(result), marker)
		})

		t.Run("AllSources", func(t *testing.T) {
			result := callTool(t, c, "logs", map[string]any{
				"sources": "messages,warnings," +
					"backtrace,compile_log," +
					"async_compile_log," +
					"init_log,stderr",
			})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			text := firstText(result)
			for _, src := range []string{
				"messages", "warnings", "backtrace",
				"compile_log", "async_compile_log",
				"init_log", "stderr",
			} {
				assert.Contains(t, text, "=== "+src+" ===")
			}
		})
	})

	t.Run("Eval", func(t *testing.T) {
		t.Run("Simple", func(t *testing.T) {
			result := callTool(t, c, "eval",
				map[string]any{
					"expression": "(+ 1 2)",
				})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			assert.Contains(t, firstText(result), "3")
		})

		t.Run("String", func(t *testing.T) {
			result := callTool(t, c, "eval",
				map[string]any{
					"expression": `(concat "hel" "lo")`,
				})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			assert.Equal(t, "hello", firstText(result))
		})

		t.Run("Version", func(t *testing.T) {
			result := callTool(t, c, "eval",
				map[string]any{
					"expression": "emacs-version",
				})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			text := firstText(result)
			hasDigit := strings.IndexAny(text, "0123456789") >= 0
			hasDot := strings.Contains(text, ".")
			assert.True(t, hasDigit && hasDot,
				"emacs-version = %q, expected version string",
				text)
		})

		t.Run("ShellCommand", func(t *testing.T) {
			result := callTool(t, c, "eval",
				map[string]any{
					"expression": `(shell-command-to-string "echo hello")`,
				})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			assert.Equal(t, "hello\n", firstText(result))
		})

		t.Run("Tty", func(t *testing.T) {
			result := callTool(t, c, "eval",
				map[string]any{
					"expression": `(shell-command-to-string (format "ps -o tty= -p %d" (emacs-pid)))`,
				})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			out := strings.TrimSpace(firstText(result))
			assert.NotEqual(t, "?", out, "emacs has no controlling tty")
			assert.NotEmpty(t, out, "emacs has no controlling tty")
		})
	})

	t.Run("Bytecomp", func(t *testing.T) {
		elFile := "/tmp/e2e-bytecomp-test.el"
		elContent := `;;; -*- lexical-binding: t -*-
(defun e2e-test-fn ()
  free-variable-ref)
(e2e-test-fn 42)
`
		writeCmd := `printf '%s' ` + `'` + elContent + `'` + ` > ` + elFile
		writeResult := callTool(t, c, "shell",
			map[string]any{"command": writeCmd})
		assert.False(t, writeResult.IsError,
			"expected success, got error: %s",
			firstText(writeResult))

		t.Run("NoFilter", func(t *testing.T) {
			result := callTool(t, c, "bytecomp",
				map[string]any{
					"file_path": elFile,
				})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			text := firstText(result)
			assert.Contains(t, text, `"file"`)
			assert.Contains(t, text, `"diagnostics"`)
			assert.Contains(t, text, `"summary"`)
			assert.NotContains(t, text, `"total":0`)
		})

		t.Run("FilterWarning", func(t *testing.T) {
			result := callTool(t, c, "bytecomp",
				map[string]any{
					"file_path": elFile,
					"severity":  "warning",
				})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			assert.NotContains(t, firstText(result), `"severity":"error"`)
		})

		t.Run("FilterError", func(t *testing.T) {
			result := callTool(t, c, "bytecomp",
				map[string]any{
					"file_path": elFile,
					"severity":  "error",
				})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			assert.NotContains(t, firstText(result), `"severity":"warning"`)
		})

		t.Run("NonexistentFile", func(t *testing.T) {
			result := callTool(t, c, "bytecomp",
				map[string]any{
					"file_path": "/tmp/" + "e2e-bytecomp-no-exist.el",
				})
			assert.True(t, result.IsError,
				"expected error result, got success: %s",
				firstText(result))
		})
	})

	t.Run("Shell", func(t *testing.T) {
		t.Run("Echo", func(t *testing.T) {
			result := callTool(t, c, "shell",
				map[string]any{
					"command": "echo hello",
				})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			assert.Contains(t, firstText(result), "hello")
		})

		t.Run("Display", func(t *testing.T) {
			result := callTool(t, c, "shell",
				map[string]any{
					"command": "echo $DISPLAY",
				})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			assert.Contains(t, firstText(result), ":")
		})

		t.Run("Tty", func(t *testing.T) {
			result := callTool(t, c, "shell",
				map[string]any{
					"command": "tty",
				})
			assert.False(t, result.IsError,
				"expected success, got error: %s",
				firstText(result))
			out := strings.TrimSpace(firstText(result))
			assert.True(t, strings.HasPrefix(out, "/dev/"),
				"expected tty device path, got %q", out)
		})
	})

	t.Run("Screenshot", func(t *testing.T) {
		result := callTool(t, c, "screenshot", nil)
		assert.False(t, result.IsError,
			"expected success, got error: %s",
			firstText(result))

		data := firstImageData(result)
		require.NotEmpty(t, data, "no image data returned")

		decoded, err := base64.StdEncoding.DecodeString(data)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(decoded), 8,
			"data too short: %d bytes", len(decoded))

		const screenshotPNGMagic = "\x89PNG"
		assert.Equal(t, screenshotPNGMagic,
			string(decoded[:4]),
			"not a PNG: first 4 bytes = %x", decoded[:4])
	})

	t.Run("Control/StatusWhileRunning", func(t *testing.T) {
		result := callTool(t, c, "control",
			map[string]any{"action": "status"})
		assert.False(t, result.IsError,
			"expected success, got error: %s",
			firstText(result))
		assert.Contains(t, firstText(result), "running")
	})

	t.Run("Control/Restart", func(t *testing.T) {
		result := callToolLong(t, c, "control",
			map[string]any{"action": "restart"},
			startTimeout)
		assert.False(t, result.IsError,
			"expected success, got error: %s",
			firstText(result))
		assert.Contains(t, firstText(result), "restarted")
	})

	t.Run("Eval/AfterRestart", func(t *testing.T) {
		result := callTool(t, c, "eval",
			map[string]any{"expression": "(+ 2 3)"})
		assert.False(t, result.IsError,
			"expected success, got error: %s",
			firstText(result))
		assert.Contains(t, firstText(result), "5")
	})

	t.Run("Control/Stop", func(t *testing.T) {
		result := callTool(t, c, "control", map[string]any{"action": "stop"})
		assert.False(t, result.IsError,
			"expected success, got error: %s",
			firstText(result))
		assert.Contains(t, firstText(result), "stopped")
	})

	t.Run("EvalAfterStop", func(t *testing.T) {
		result := callTool(t, c, "eval",
			map[string]any{"expression": "(+ 1 2)"})
		assert.True(t, result.IsError,
			"expected error result, got success: %s",
			firstText(result))
		assert.Contains(t, firstText(result), "not running")
	})

	t.Run("Control/StatusAfterStop", func(t *testing.T) {
		result := callTool(t, c, "control",
			map[string]any{"action": "status"})
		assert.False(t, result.IsError,
			"expected success, got error: %s",
			firstText(result))
		assert.Contains(t, firstText(result), "stopped")
	})
}

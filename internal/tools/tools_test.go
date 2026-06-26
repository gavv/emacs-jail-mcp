// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gavv/emacs-jail-mcp/internal/config"
	"github.com/gavv/emacs-jail-mcp/internal/jail"
	"github.com/gavv/emacs-jail-mcp/internal/tools"
)

// newTestServer creates an MCPServer with all jail tools registered.
func newTestServer() (*server.MCPServer, *jail.Jail) {
	cfg := config.DefaultServer()
	sb := jail.New(cfg)
	s := server.NewMCPServer("test", "0.0.0", server.WithToolCapabilities(true))
	tools.Register(s, sb)
	return s, sb
}

// listTools calls tools/list and returns the list of registered tools.
func listTools(t *testing.T, s *server.MCPServer) []mcp.Tool {
	t.Helper()
	msg := s.HandleMessage(context.Background(), json.RawMessage(
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
	))
	resp, ok := msg.(mcp.JSONRPCResponse)
	require.True(t, ok, "HandleMessage returned unexpected type %T", msg)
	result, ok := resp.Result.(mcp.ListToolsResult)
	require.True(t, ok, "tools/list result is %T, not ListToolsResult", resp.Result)
	return result.Tools
}

// callTool calls tools/call and returns the CallToolResult.
func callTool(t *testing.T, s *server.MCPServer, name string, args map[string]any,
) mcp.CallToolResult {
	t.Helper()

	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)

	payload := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		name, argsJSON,
	)

	msg := s.HandleMessage(context.Background(), json.RawMessage(payload))
	resp, ok := msg.(mcp.JSONRPCResponse)
	require.True(t, ok, "HandleMessage for %q returned unexpected type %T", name, msg)
	result, ok := resp.Result.(mcp.CallToolResult)
	require.True(t, ok, "tools/call result for %q is %T, not CallToolResult", name, resp.Result)
	return result
}

// firstText extracts the first TextContent text from a CallToolResult.
func firstText(result mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func TestRegisterAllTools(t *testing.T) {
	s, _ := newTestServer()
	registered := listTools(t, s)

	expected := []string{
		"emacs_jail_control",
		"emacs_jail_logs",
		"emacs_jail_eval",
		"emacs_jail_shell",
		"emacs_jail_screenshot",
		"emacs_jail_bytecomp",
	}

	assert.Len(t, registered, len(expected))
	nameSet := make(map[string]bool)
	for _, tool := range registered {
		nameSet[tool.Name] = true
	}
	for _, name := range expected {
		assert.True(t, nameSet[name], "tool %q not registered", name)
	}
}

func TestControlToolHasActionParam(t *testing.T) {
	s, _ := newTestServer()
	for _, tool := range listTools(t, s) {
		if tool.Name == "emacs_jail_control" {
			_, ok := tool.InputSchema.Properties["action"]
			assert.True(t, ok)
			return
		}
	}
	t.Error("emacs_jail_control not found")
}

func TestEvalToolHasExpressionParam(t *testing.T) {
	s, _ := newTestServer()
	for _, tool := range listTools(t, s) {
		if tool.Name == "emacs_jail_eval" {
			_, ok := tool.InputSchema.Properties["expression"]
			assert.True(t, ok)
			return
		}
	}
	t.Error("emacs_jail_eval not found")
}

func TestLogsToolHasSourcesParam(t *testing.T) {
	s, _ := newTestServer()
	for _, tool := range listTools(t, s) {
		if tool.Name == "emacs_jail_logs" {
			_, ok := tool.InputSchema.Properties["sources"]
			assert.True(t, ok)
			_, ok = tool.InputSchema.Properties["offset"]
			assert.True(t, ok)
			_, ok = tool.InputSchema.Properties["limit"]
			assert.True(t, ok)
			return
		}
	}
	t.Error("emacs_jail_logs not found")
}

func TestShellToolHasCommandParam(t *testing.T) {
	s, _ := newTestServer()
	for _, tool := range listTools(t, s) {
		if tool.Name == "emacs_jail_shell" {
			_, ok := tool.InputSchema.Properties["command"]
			assert.True(t, ok)
			return
		}
	}
	t.Error("emacs_jail_shell not found")
}

func TestControlStopWhenNotRunning(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_control", map[string]any{"action": "stop"})
	assert.True(t, result.IsError, "expected IsError=true; text: %q", firstText(result))
	assert.Contains(t, firstText(result), "not running")
}

func TestControlRestartWhenNotRunning(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_control", map[string]any{"action": "restart"})
	assert.True(t, result.IsError, "expected IsError=true; text: %q", firstText(result))
	assert.Contains(t, firstText(result), "not running")
}

func TestControlStatusWhenStopped(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_control", map[string]any{"action": "status"})
	assert.False(t, result.IsError, "status should not be an error; got: %q", firstText(result))
	assert.Contains(t, firstText(result), "stopped")
}

func TestControlInvalidAction(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_control", map[string]any{"action": "invalid"})
	assert.True(t, result.IsError, "expected IsError=true; text: %q", firstText(result))
	assert.Contains(t, firstText(result), "unknown action")
}

func TestEvalWhenNotRunning(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_eval", map[string]any{"expression": "(+ 1 2)"})
	assert.True(t, result.IsError, "expected IsError=true; text: %q", firstText(result))
	assert.Contains(t, firstText(result), "not running")
}

func TestShellWhenNotRunning(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_shell", map[string]any{"command": "echo hi"})
	assert.True(t, result.IsError, "expected IsError=true; text: %q", firstText(result))
	assert.Contains(t, firstText(result), "not running")
}

func TestScreenshotWhenNotRunning(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_screenshot", nil)
	assert.True(t, result.IsError, "expected IsError=true; text: %q", firstText(result))
	assert.Contains(t, firstText(result), "not running")
}

func TestLogsInitLogEmptyWhenNoOutput(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_logs", map[string]any{"sources": "init_log"})
	assert.False(t, result.IsError,
		"emacs_jail_logs should not return an error; got: %q", firstText(result))
	text := firstText(result)
	assert.Contains(t, text, "=== init_log ===")
	assert.Contains(t, text, "(empty)")
}

func TestLogsStderrEmptyWhenNoOutput(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_logs", map[string]any{"sources": "stderr"})
	assert.False(t, result.IsError,
		"emacs_jail_logs should not return an error; got: %q", firstText(result))
	assert.Contains(t, firstText(result), "=== stderr ===")
}

func TestLogsUnknownSourceReturnsError(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_logs", map[string]any{"sources": "nosuchsource"})
	assert.True(t, result.IsError, "expected IsError=true; text: %q", firstText(result))
	assert.Contains(t, firstText(result), "unknown source")
}

func TestLogsDefaultSourcesWhenNotRunning(t *testing.T) {
	// Default sources are buffer-based; when jail is not running, each section
	// contains an error message but the overall result is not IsError.
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_logs", nil)
	assert.False(t, result.IsError,
		"emacs_jail_logs should not return IsError for default sources; got: %q",
		firstText(result))
	assert.Contains(t, firstText(result), "=== messages ===")
}

func TestEvalMissingExpression(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_eval", map[string]any{"expression": ""})
	assert.True(t, result.IsError, "expected IsError=true; text: %q", firstText(result))
}

func TestShellMissingCommand(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_shell", map[string]any{"command": ""})
	assert.True(t, result.IsError, "expected IsError=true; text: %q", firstText(result))
}

func TestBytecompToolHasFilePathAndSeverityParams(t *testing.T) {
	s, _ := newTestServer()
	for _, tool := range listTools(t, s) {
		if tool.Name == "emacs_jail_bytecomp" {
			_, ok := tool.InputSchema.Properties["file_path"]
			assert.True(t, ok)
			_, ok = tool.InputSchema.Properties["severity"]
			assert.True(t, ok)
			return
		}
	}
	t.Error("emacs_jail_bytecomp not found")
}

func TestBytecompWhenNotRunning(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_bytecomp", map[string]any{
		"file_path": "/tmp/test.el",
	})
	assert.True(t, result.IsError, "expected IsError=true; text: %q", firstText(result))
	assert.Contains(t, firstText(result), "not running")
}

func TestBytecompMissingFilePath(t *testing.T) {
	s, _ := newTestServer()
	result := callTool(t, s, "emacs_jail_bytecomp", map[string]any{"file_path": ""})
	assert.True(t, result.IsError, "expected IsError=true; text: %q", firstText(result))
}

// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

//go:build e2e

package e2e_test

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gavv/emacs-jail-mcp/internal/cli"
	"github.com/gavv/emacs-jail-mcp/internal/config"
)

// TestServer manages the lifecycle of an in-process MCP server for e2e tests.
type TestServer struct {
	stdinW    *os.File
	origStdin *os.File
	errCh     <-chan error
}

func (s *TestServer) Addr() string {
	return fmt.Sprintf("127.0.0.1:%d", config.E2ETestMCPPort)
}

func (s *TestServer) Port() string {
	return fmt.Sprintf("%d", config.E2ETestMCPPort)
}

// Start launches the MCP server via the cobra command in a goroutine
// with piped stdin. It polls the TCP port until the server is accepting
// connections.
func (s *TestServer) Start(t *testing.T) {
	t.Helper()

	stdinR, stdinW, err := os.Pipe()
	require.NoError(t, err)

	s.stdinW = stdinW
	s.origStdin = os.Stdin
	os.Stdin = stdinR

	errCh := make(chan error, 1)
	s.errCh = errCh
	go func() {
		cmd := cli.NewRootCmd()
		cmd.SetArgs([]string{
			"serve",
			"--mcp-port", fmt.Sprintf("%d", config.E2ETestMCPPort),
		})
		errCh <- cmd.Execute()
	}()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", s.Addr(), time.Second)
		if dialErr == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server at %s not ready after 10s", s.Addr())
}

// Stop closes stdin to signal the server to exit and waits for it to
// shut down cleanly.
func (s *TestServer) Stop(t *testing.T) {
	t.Helper()

	_ = s.stdinW.Close()
	os.Stdin = s.origStdin

	select {
	case err := <-s.errCh:
		assert.NoError(t, err, "server exited with error")
	case <-time.After(10 * time.Second):
		t.Error("server did not exit within 10s")
	}
}

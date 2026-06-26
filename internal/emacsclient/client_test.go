// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package emacsclient_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gavv/emacs-jail-mcp/internal/emacsclient"
)

// mockServer is a minimal Unix socket server for testing the client.
type mockServer struct {
	listener net.Listener
	path     string
}

func newMockServer(t *testing.T) *mockServer {
	t.Helper()
	path := fmt.Sprintf("/tmp/emacs-client-test-%d.sock", os.Getpid())
	_ = os.Remove(path)

	l, err := net.Listen("unix", path)
	require.NoError(t, err)

	return &mockServer{listener: l, path: path}
}

func (s *mockServer) close() {
	_ = s.listener.Close()
	_ = os.Remove(s.path)
}

// acceptOne accepts a single connection and runs the given handler in a goroutine.
func (s *mockServer) acceptOne(handler func(net.Conn)) {
	go func() {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		handler(conn)
	}()
}

// evalHandler reads JSON-RPC eval requests and responds with the expression
// wrapped in a result object, mimicking emacs-jail-rpc.el behaviour.
func evalHandler(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	for {
		var req emacsclient.Request
		if err := dec.Decode(&req); err != nil {
			return
		}
		id := req.ID
		resp := emacsclient.Response{
			JSONRPC: "2.0",
			ID:      &id,
			Result:  json.RawMessage(`{"value":"ok"}`),
		}
		if err := enc.Encode(resp); err != nil {
			return
		}
	}
}

func TestClientConnect(t *testing.T) {
	srv := newMockServer(t)
	defer srv.close()
	srv.acceptOne(func(c net.Conn) { _ = c.Close() })

	c := emacsclient.NewClient(srv.path)
	require.NoError(t, c.Connect(context.Background()))
	defer func() { _ = c.Close() }()
}

func TestClientConnectNoServer(t *testing.T) {
	c := emacsclient.NewClient("/tmp/emacs-client-test-nonexistent.sock")
	err := c.Connect(context.Background())
	assert.Error(t, err)
	if err == nil {
		_ = c.Close()
	}
}

func TestClientEvalElisp(t *testing.T) {
	srv := newMockServer(t)
	defer srv.close()
	srv.acceptOne(evalHandler)

	c := emacsclient.NewClient(srv.path)
	require.NoError(t, c.Connect(context.Background()))
	defer func() { _ = c.Close() }()

	result, err := c.EvalElisp(context.Background(), "(+ 1 2)")
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

func TestClientContextCancellation(t *testing.T) {
	srv := newMockServer(t)
	defer srv.close()
	srv.acceptOne(func(c net.Conn) {
		// Hang without responding.
		time.Sleep(10 * time.Second)
		_ = c.Close()
	})

	c := emacsclient.NewClient(srv.path)
	require.NoError(t, c.Connect(context.Background()))
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.EvalElisp(ctx, "(+ 1 2)")
	assert.Error(t, err)
}

func TestClientMalformedResponse(t *testing.T) {
	srv := newMockServer(t)
	defer srv.close()

	srv.acceptOne(func(conn net.Conn) {
		defer func() { _ = conn.Close() }()
		// Read the request, then send back malformed JSON.
		buf := make([]byte, 512)
		_, _ = conn.Read(buf)
		_, _ = conn.Write([]byte("not-json\n"))
	})

	c := emacsclient.NewClient(srv.path)
	require.NoError(t, c.Connect(context.Background()))
	defer func() { _ = c.Close() }()

	_, err := c.EvalElisp(context.Background(), "(+ 1 2)")
	assert.Error(t, err)
}

func TestClientSequentialRequests(t *testing.T) {
	srv := newMockServer(t)
	defer srv.close()
	srv.acceptOne(evalHandler)

	c := emacsclient.NewClient(srv.path)
	require.NoError(t, c.Connect(context.Background()))
	defer func() { _ = c.Close() }()

	for i := 0; i < 5; i++ {
		result, err := c.EvalElisp(context.Background(), fmt.Sprintf("(+ %d 0)", i))
		require.NoError(t, err, "iteration %d", i)
		assert.Equal(t, "ok", result, "iteration %d", i)
	}
}

func TestClientConcurrentRequests(t *testing.T) {
	srv := newMockServer(t)
	defer srv.close()
	srv.acceptOne(evalHandler)

	c := emacsclient.NewClient(srv.path)
	require.NoError(t, c.Connect(context.Background()))
	defer func() { _ = c.Close() }()

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	results := make([]string, n)

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = c.EvalElisp(
				context.Background(), fmt.Sprintf("(+ %d 0)", idx))
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		assert.NoError(t, errs[i], "goroutine %d", i)
		assert.Equal(t, "ok", results[i], "goroutine %d", i)
	}
}

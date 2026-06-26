// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package emacsclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	logging "github.com/op/go-logging"
)

var log = logging.MustGetLogger("emacsclient")

type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

type ResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Client connects to the Emacs Jail RPC server over a Unix socket.
// Requests are sent and received sequentially — emacs-jail-rpc processes one
// request at a time, so no concurrent dispatch is needed.
type Client struct {
	socketPath string
	conn       net.Conn
	reader     *bufio.Reader
	nextID     atomic.Int64
	mu         sync.Mutex
}

func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

// The client is ready to send eval requests immediately after Connect returns.
func (c *Client) Connect(_ context.Context) error {
	log.Infof("connecting to emacs at %s", c.socketPath)

	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		log.Errorf("failed to connect to emacs at %s: %v", c.socketPath, err)
		return fmt.Errorf("dial unix %q: %w", c.socketPath, err)
	}
	c.conn = conn
	c.reader = bufio.NewReader(conn)

	log.Infof("connected to emacs")
	return nil
}

// If the Elisp expression itself raises an error, EvalElisp returns a non-nil Go
// error wrapping the Elisp error message.
func (c *Client) EvalElisp(ctx context.Context, expr string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		log.Errorf("eval called but client not connected")
		return "", fmt.Errorf("client not connected")
	}

	id := c.nextID.Add(1)
	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "eval",
		Params:  map[string]string{"expression": expr},
	}

	log.Debugf("eval request id=%d expr=%s", id, truncateExpr(expr, 100))

	data, err := json.Marshal(req)
	if err != nil {
		log.Errorf("failed to marshal request: %v", err)
		return "", fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')

	// Respect context cancellation for the write.
	if err := ctx.Err(); err != nil {
		log.Warningf("eval cancelled before write id=%d: %v", id, err)
		return "", err
	}

	if _, err := c.conn.Write(data); err != nil {
		log.Errorf("failed to write request id=%d: %v", id, err)
		return "", fmt.Errorf("write request: %w", err)
	}

	// Read the response, unblocking early if ctx is cancelled.
	type readResult struct {
		line []byte
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		line, err := c.reader.ReadBytes('\n')
		ch <- readResult{line, err}
	}()

	var rr readResult
	select {
	case rr = <-ch:
	case <-ctx.Done():
		log.Warningf("eval cancelled during read id=%d: %v", id, ctx.Err())
		// Close the connection so the background reader unblocks.
		_ = c.conn.Close()
		c.conn = nil
		return "", ctx.Err()
	}

	if rr.err != nil {
		log.Errorf("failed to read response id=%d: %v", id, rr.err)
		return "", fmt.Errorf("read response: %w", rr.err)
	}

	var resp Response
	if err := json.Unmarshal(rr.line, &resp); err != nil {
		log.Errorf("failed to unmarshal response id=%d: %v", id, err)
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	// JSON-RPC protocol error (e.g. method not found, parse error).
	if resp.Error != nil {
		log.Errorf("eval rpc error id=%d code=%d msg=%s", id, resp.Error.Code, resp.Error.Message)
		return "", fmt.Errorf(
			"eval rpc error [%d]: %s",
			resp.Error.Code, resp.Error.Message)
	}

	// evalResult is the JSON shape of a successful eval response.
	var result struct {
		Value string `json:"value,omitempty"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		log.Errorf("failed to unmarshal eval result id=%d: %v", id, err)
		return "", fmt.Errorf("unmarshal eval result: %w", err)
	}

	// Application-level eval error (Elisp raised a condition).
	if result.Error != "" {
		log.Warningf("eval error id=%d: %s", id, result.Error)
		return "", fmt.Errorf("%s", result.Error)
	}

	log.Debugf("eval response id=%d value=%s", id, truncateExpr(result.Value, 100))
	return result.Value, nil
}

func (c *Client) Close() error {
	log.Infof("closing emacs client connection")
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

func truncateExpr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return fmt.Sprintf("%s...", s[:n])
}

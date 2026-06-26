// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package jail

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gavv/emacs-jail-mcp/internal/config"
	"github.com/gavv/emacs-jail-mcp/internal/display"
)

// mockBackend is a test implementation of containerBackend.
type mockBackend struct {
	StartErr   error
	StopErr    error
	ExecOut    string
	ExecErr    error
	ExecRawOut []byte
	ExecRawErr error
	Running    bool
	Lines      []string
	StderrOut  []string
}

func (m *mockBackend) Start(_ context.Context) error {
	if m.StartErr != nil {
		return m.StartErr
	}
	m.Running = true
	return nil
}

func (m *mockBackend) Stop(_ context.Context) error {
	if m.StopErr != nil {
		return m.StopErr
	}
	m.Running = false
	return nil
}

func (m *mockBackend) Exec(_ context.Context, _ string, _ ...string) (string, error) {
	return m.ExecOut, m.ExecErr
}

func (m *mockBackend) ExecRaw(_ context.Context, _ string) ([]byte, error) {
	return m.ExecRawOut, m.ExecRawErr
}

func (m *mockBackend) IsRunning() bool {
	return m.Running
}

func (m *mockBackend) LogLines(_ int) []string {
	return m.Lines
}

func (m *mockBackend) StderrLines(_ int) []string {
	return m.StderrOut
}

// mockClient is a test implementation of emacsClient.
type mockClient struct {
	ConnectErr   error
	EvalOut      string
	EvalErr      error
	CloseErr     error
	ConnectCalls int
	CloseCalls   int
}

func (m *mockClient) Connect(_ context.Context) error {
	m.ConnectCalls++
	return m.ConnectErr
}

func (m *mockClient) EvalElisp(_ context.Context, _ string) (string, error) {
	return m.EvalOut, m.EvalErr
}

func (m *mockClient) Close() error {
	m.CloseCalls++
	return m.CloseErr
}

// newMockJail creates a Jail backed by mock backend and client.
func newMockJail(cfg *config.ServerConfig) (*Jail, *mockBackend, *mockClient) {
	if cfg == nil {
		cfg = config.DefaultServer()
	}
	b := &mockBackend{}
	c := &mockClient{}
	jl := &Jail{
		serverConfig: cfg,
		display:      display.Default(20),
		backend:      b,
		client:       c,
		state:        StateStopped,
	}
	return jl, b, c
}

// newTestJail creates a jail with default config for tests.
func newTestJail() *Jail {
	cfg := config.DefaultServer()
	return New(cfg)
}

func TestInitialState(t *testing.T) {
	jl := newTestJail()
	assert.False(t, jl.IsRunning())
	assert.Equal(t, StateStopped, jl.CurrentState())
}

func TestStopWhenStopped(t *testing.T) {
	jl := newTestJail()
	err := jl.Stop(context.Background())
	assert.Error(t, err)
}

func TestEvalWhenStopped(t *testing.T) {
	jl := newTestJail()
	_, err := jl.EvalElisp(context.Background(), "(+ 1 2)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestShellWhenStopped(t *testing.T) {
	jl := newTestJail()
	_, err := jl.Shell(context.Background(), "echo hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestScreenshotBase64WhenStopped(t *testing.T) {
	jl := newTestJail()
	_, err := jl.ScreenshotBase64(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

// TestStartFailsWhenContainerUnavailable verifies Start transitions back to
// Stopped when the container cannot be launched.
func TestStartFailsWhenContainerUnavailable(t *testing.T) {
	cfg := config.DefaultServer()
	cfg.PodmanBinary = "/nonexistent/podman"
	cfg.UseSudo = false
	cfg.StartTimeout = 1 * time.Second

	jl := New(cfg)
	startErr := jl.Start(context.Background())

	assert.Error(t, startErr)
	assert.Equal(t, StateStopped, jl.CurrentState())
	assert.False(t, jl.IsRunning())
}

// TestStartAlreadyStarting verifies Start returns an error when state is Starting.
func TestStartAlreadyStarting(t *testing.T) {
	jl := newTestJail()
	jl.state = StateStarting
	err := jl.Start(context.Background())
	assert.Error(t, err)
}

func TestStartAlreadyRunning(t *testing.T) {
	jl := newTestJail()
	jl.state = StateRunning

	err := jl.Start(context.Background())
	assert.Error(t, err)
	assert.Equal(t, StateRunning, jl.CurrentState())
}

// listenUnix creates a Unix socket listener at the given path and returns it.
// The caller is responsible for closing the listener and removing the file.
func listenUnix(t *testing.T, path string) net.Listener {
	t.Helper()
	_ = os.Remove(path)
	l, err := net.Listen("unix", path)
	require.NoError(t, err, "listen unix %q", path)
	return l
}

// newMockCfg returns a ServerConfig with a temp-dir socket dir, suitable for mock tests.
func newMockCfg(t *testing.T) *config.ServerConfig {
	t.Helper()
	cfg := config.DefaultServer()
	cfg.EmacsSocketDir = t.TempDir()
	cfg.ContainerPrefix = "mock-" + t.Name()
	cfg.StartTimeout = 2 * time.Second
	cfg.StopTimeout = 2 * time.Second
	return cfg
}

func TestMockStartStop(t *testing.T) {
	cfg := newMockCfg(t)
	jl, backend, client := newMockJail(cfg)

	l := listenUnix(t, cfg.SocketPath())
	defer func() {
		_ = l.Close()
		_ = os.Remove(cfg.SocketPath())
	}()
	go func() { _, _ = l.Accept() }()

	require.NoError(t, jl.Start(context.Background()))

	assert.True(t, jl.IsRunning())
	assert.Equal(t, StateRunning, jl.CurrentState())
	assert.Equal(t, 1, client.ConnectCalls)
	assert.True(t, backend.Running)

	require.NoError(t, jl.Stop(context.Background()))

	assert.False(t, jl.IsRunning())
	assert.Equal(t, StateStopped, jl.CurrentState())
	assert.Equal(t, 1, client.CloseCalls)
}

func TestMockStartBackendFailure(t *testing.T) {
	cfg := newMockCfg(t)
	jl, backend, _ := newMockJail(cfg)
	backend.StartErr = fmt.Errorf("jail is not running")

	err := jl.Start(context.Background())
	require.Error(t, err)

	assert.Equal(t, StateStopped, jl.CurrentState())
	assert.False(t, jl.IsRunning())
}

func TestMockEvalDelegates(t *testing.T) {
	jl, _, client := newMockJail(nil)
	client.EvalOut = "42"
	jl.state = StateRunning

	result, err := jl.EvalElisp(context.Background(), "(+ 1 2)")
	require.NoError(t, err)
	assert.Equal(t, "42", result)
}

func TestMockEvalError(t *testing.T) {
	jl, _, client := newMockJail(nil)
	client.EvalErr = fmt.Errorf("jail is not running")
	jl.state = StateRunning

	_, err := jl.EvalElisp(context.Background(), "(foo)")
	assert.Error(t, err)
}

func TestMockShellDelegates(t *testing.T) {
	jl, backend, _ := newMockJail(nil)
	backend.ExecOut = "hello\n"
	jl.state = StateRunning

	out, err := jl.Shell(context.Background(), "echo hello")
	require.NoError(t, err)
	assert.Equal(t, "hello\n", out)
}

func TestMockScreenshotBase64Delegates(t *testing.T) {
	jl, backend, _ := newMockJail(nil)
	backend.ExecRawOut = []byte("PNG")
	jl.state = StateRunning

	result, err := jl.ScreenshotBase64(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	// base64("PNG") = "UE5H"
	assert.Equal(t, "UE5H", result)
}

func TestReadBufferWhenNotRunning(t *testing.T) {
	jl := newTestJail()
	_, err := jl.ReadBuffer(context.Background(), "*Messages*")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestReadBufferDelegates(t *testing.T) {
	jl, _, client := newMockJail(nil)
	client.EvalOut = "hello from buffer"
	jl.state = StateRunning

	result, err := jl.ReadBuffer(context.Background(), "*Messages*")
	require.NoError(t, err)
	assert.Equal(t, "hello from buffer", result)
}

func TestRestartWhenStopped(t *testing.T) {
	jl := newTestJail()
	err := jl.Restart(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestRestartStopFails(t *testing.T) {
	jl, _, _ := newMockJail(nil)
	jl.state = StateRunning
	failing := &mockBackend{StopErr: fmt.Errorf("kill failed")}
	jl.backend = failing

	err := jl.Restart(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stop failed")
}

func TestRestartStartFails(t *testing.T) {
	cfg := newMockCfg(t)
	jl, backend, _ := newMockJail(cfg)
	// Set jail directly to Running state (skip actual Start).
	jl.state = StateRunning
	jl.logLines = []string{"old log line"}
	// Backend stop succeeds, but second start (after stop) will fail.
	backend.StartErr = fmt.Errorf("container launch error")

	err := jl.Restart(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start failed")
	assert.Equal(t, StateStopped, jl.CurrentState())
	// Logs must have been cleared even though start failed.
	assert.Empty(t, jl.LogLines())
}

func TestMockRestart(t *testing.T) {
	cfg := newMockCfg(t)
	jl, backend, client := newMockJail(cfg)

	l := listenUnix(t, cfg.SocketPath())
	defer func() {
		_ = l.Close()
		_ = os.Remove(cfg.SocketPath())
	}()
	go func() {
		for {
			_, _ = l.Accept()
		}
	}()

	// Start to get into Running state.
	require.NoError(t, jl.Start(context.Background()))

	// Plant some log lines to verify they are cleared on Restart.
	jl.logLines = []string{"old line 1", "old line 2"}

	closeBefore := client.CloseCalls

	require.NoError(t, jl.Restart(context.Background()))

	assert.True(t, jl.IsRunning())
	assert.Equal(t, StateRunning, jl.CurrentState())
	// Stop must have closed the original client.
	assert.Equal(t, closeBefore+1, client.CloseCalls)
	// Backend must have been cycled: stopped then started again.
	assert.True(t, backend.Running)
	// Old log lines must be gone.
	for _, line := range jl.LogLines() {
		assert.NotContains(t, []string{"old line 1", "old line 2"}, line,
			"old log line %q still present after Restart", line)
	}
}

// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package jail

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	logging "github.com/op/go-logging"

	"github.com/gavv/emacs-jail-mcp/internal/config"
	"github.com/gavv/emacs-jail-mcp/internal/container"
	"github.com/gavv/emacs-jail-mcp/internal/display"
	"github.com/gavv/emacs-jail-mcp/internal/emacsclient"
)

var (
	log      = logging.MustGetLogger("jail")
	emacsLog = logging.MustGetLogger("emacs")
)

var errNotRunning = errors.New(
	"jail is not running;" +
		" call control with action=start first")

type State int

const (
	StateStopped State = iota
	StateStarting
	StateRunning
	StateStopping
)

func (s State) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	default:
		return "unknown"
	}
}

// containerBackend abstracts the container for testability.
type containerBackend interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Exec(ctx context.Context, cmd string, env ...string) (string, error)
	ExecRaw(ctx context.Context, cmd string) ([]byte, error)
	IsRunning() bool
	LogLines(n int) []string
	StderrLines(n int) []string
}

// emacsClient abstracts the Emacs eval client for testability.
type emacsClient interface {
	Connect(ctx context.Context) error
	EvalElisp(ctx context.Context, expr string) (string, error)
	Close() error
}

// Jail orchestrates a Podman container running Emacs, Xvfb, and emacs-jail-rpc.
// All public methods are safe for concurrent use.
type Jail struct {
	mu          sync.RWMutex
	serverConfig *config.ServerConfig
	display     *display.Display
	backend     containerBackend
	client      emacsClient
	state       State
	logLines    []string
	stderrLines []string
	stderrTail  *stderrStreamer
}

func New(cfg *config.ServerConfig) *Jail {
	disp := display.New(cfg.Display)
	return &Jail{
		serverConfig: cfg,
		display:      disp,
		backend:      container.New(cfg, disp),
		state:        StateStopped,
	}
}

func (j *Jail) Start(ctx context.Context) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	return j.startLocked(ctx)
}

func (j *Jail) startLocked(ctx context.Context) error {
	if j.state != StateStopped {
		return fmt.Errorf("jail is already %s", j.state)
	}

	log.Infof("starting jail (state: %s -> starting)", j.state)
	j.state = StateStarting

	if err := j.backend.Start(ctx); err != nil {
		j.state = StateStopped
		log.Errorf("container start failed: %v", err)
		return fmt.Errorf("start container: %w", err)
	}

	log.Infof("starting emacs stderr streaming from %s", j.serverConfig.StderrPath())
	j.stderrTail = startStderrStreamer(context.Background(), j.serverConfig.StderrPath(),
		func(line string) {
			emacsLog.Debugf("%s", line)
		})

	startCtx, cancel := context.WithTimeout(ctx, j.serverConfig.StartTimeout)
	defer cancel()

	log.Infof("waiting for emacs-jail-rpc socket at %s", j.serverConfig.SocketPath())
	if err := j.waitForSocket(startCtx); err != nil {
		j.stderrTail.stop()
		j.stderrTail = nil
		j.state = StateStopped
		_ = j.backend.Stop(context.Background())
		log.Errorf("socket wait failed: %v", err)
		return fmt.Errorf("wait for emacs-jail-rpc socket: %w", err)
	}

	if j.client == nil {
		j.client = emacsclient.NewClient(j.serverConfig.SocketPath())
	}

	log.Infof("connecting emacs client to %s", j.serverConfig.SocketPath())
	if err := j.client.Connect(startCtx); err != nil {
		j.stderrTail.stop()
		j.stderrTail = nil
		j.state = StateStopped
		_ = j.client.Close()
		j.client = nil
		_ = j.backend.Stop(context.Background())
		log.Errorf("emacs client connect failed: %v", err)
		return fmt.Errorf("connect to emacs: %w", err)
	}

	j.state = StateRunning
	log.Infof("jail is now running")
	return nil
}

func (j *Jail) Stop(ctx context.Context) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	return j.stopLocked(ctx)
}

func (j *Jail) stopLocked(ctx context.Context) error {
	if j.state != StateRunning {
		return fmt.Errorf("jail is not running (state: %s)", j.state)
	}

	log.Infof("stopping jail (state: %s -> stopping)", j.state)
	j.state = StateStopping

	var firstErr error

	if j.stderrTail != nil {
		j.stderrTail.stop()
		j.stderrTail = nil
	}

	if j.client != nil {
		if err := j.client.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close client: %w", err)
		}
		j.client = nil
	}

	stopCtx, cancel := context.WithTimeout(ctx, j.serverConfig.StopTimeout)
	defer cancel()

	if err := j.backend.Stop(stopCtx); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("stop container: %w", err)
	}

	j.state = StateStopped

	if firstErr != nil {
		log.Errorf("jail stop had error: %v", firstErr)
	} else {
		log.Infof("jail stopped")
	}
	return firstErr
}

func (j *Jail) Restart(ctx context.Context) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.state != StateRunning {
		return fmt.Errorf("jail is not running (state: %s)", j.state)
	}
	log.Infof("restarting jail")
	if err := j.stopLocked(ctx); err != nil {
		return fmt.Errorf("restart: stop failed: %w", err)
	}
	j.logLines = nil
	j.stderrLines = nil
	if err := j.startLocked(ctx); err != nil {
		return fmt.Errorf("restart: start failed (jail is now stopped): %w", err)
	}
	log.Infof("jail restarted successfully")
	return nil
}

func (j *Jail) LogLines() []string {
	j.mu.RLock()
	defer j.mu.RUnlock()

	return j.logLines
}

func (j *Jail) StderrLines() []string {
	j.mu.RLock()
	defer j.mu.RUnlock()

	return j.stderrLines
}

func (j *Jail) EvalElisp(ctx context.Context, expr string) (string, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if j.state != StateRunning {
		log.Warningf("eval rejected: jail is %s", j.state)
		return "", errNotRunning
	}

	log.Debugf("evaluating elisp (%d bytes)", len(expr))

	evalCtx, cancel := context.WithTimeout(ctx, j.serverConfig.ExecTimeout)
	defer cancel()

	result, err := j.client.EvalElisp(evalCtx, expr)
	if err != nil {
		log.Errorf("eval failed: %v", err)
		return "", err
	}

	log.Debugf("eval completed (%d bytes result)", len(result))
	return result, nil
}

// Shell runs a shell command inside the container and returns its combined output.
// DISPLAY is passed explicitly so the command can interact with Xvfb.
func (j *Jail) Shell(ctx context.Context, cmd string) (string, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if j.state != StateRunning {
		log.Warningf("shell rejected: jail is %s", j.state)
		return "", errNotRunning
	}

	log.Debugf("running shell command: %s", cmd)

	shellCtx, cancel := context.WithTimeout(ctx, j.serverConfig.ExecTimeout)
	defer cancel()

	display := fmt.Sprintf("DISPLAY=%s", j.display.String())
	result, err := j.backend.Exec(shellCtx, cmd, display)
	if err != nil {
		log.Errorf("shell command failed: %v", err)
		return "", err
	}

	log.Debugf("shell command completed (%d bytes output)", len(result))
	return result, nil
}

// screenshot returns a raw PNG of the Xvfb display via ImageMagick import.
func (j *Jail) screenshot(ctx context.Context) ([]byte, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	if j.state != StateRunning {
		log.Warningf("screenshot rejected: jail is %s", j.state)
		return nil, errNotRunning
	}

	log.Infof("capturing screenshot")

	cmd := fmt.Sprintf(
		"import -window root -display %s png:- 2>/dev/null",
		j.display.String())

	data, err := j.backend.ExecRaw(ctx, cmd)
	if err != nil {
		log.Errorf("screenshot failed: %v", err)
		return nil, err
	}

	log.Debugf("screenshot captured (%d bytes)", len(data))
	return data, nil
}

func (j *Jail) ScreenshotBase64(ctx context.Context) (string, error) {
	raw, err := j.screenshot(ctx)
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	log.Debugf("screenshot encoded to base64 (%d bytes)", len(encoded))
	return encoded, nil
}

func (j *Jail) IsRunning() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()

	return j.state == StateRunning
}

func (j *Jail) CurrentState() State {
	j.mu.RLock()
	defer j.mu.RUnlock()

	return j.state
}

// Collects Emacs log lines while waiting.
func (j *Jail) waitForSocket(ctx context.Context) error {
	socketPath := j.serverConfig.SocketPath()

	for {
		conn, err := net.DialTimeout("unix", socketPath, time.Second)
		if err == nil {
			_ = conn.Close()
			j.collectLogLines()
			j.collectStderrLines()
			log.Infof("emacs-jail-rpc socket is ready")
			return nil
		}

		select {
		case <-ctx.Done():
			j.collectLogLines()
			j.collectStderrLines()
			return fmt.Errorf("timed out waiting for socket %q: %w", socketPath, ctx.Err())
		case <-time.After(200 * time.Millisecond):
			j.collectLogLines()
			j.collectStderrLines()
		}
	}
}

func (j *Jail) collectLogLines() {
	all := j.backend.LogLines(0) // 0 = all lines
	if len(all) > len(j.logLines) {
		j.logLines = append(j.logLines, all[len(j.logLines):]...)
	}
}

func (j *Jail) collectStderrLines() {
	all := j.backend.StderrLines(0) // 0 = all lines
	if len(all) > len(j.stderrLines) {
		j.stderrLines = append(j.stderrLines, all[len(j.stderrLines):]...)
	}
}

// Returns an error if the jail is not running or the eval fails.
func (j *Jail) ReadBuffer(ctx context.Context, bufName string) (string, error) {
	log.Infof("reading buffer %q", bufName)

	expr := fmt.Sprintf(
		`(if (get-buffer %q)`+
			` (with-current-buffer %q (buffer-string))`+
			` "")`,
		bufName, bufName)
	result, err := j.EvalElisp(ctx, expr)
	if err != nil {
		log.Errorf("failed to read buffer %q: %v", bufName, err)
		return "", err
	}

	log.Infof("read buffer %q (%d bytes)", bufName, len(result))
	return result, nil
}

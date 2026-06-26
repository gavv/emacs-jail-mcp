// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package container

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	logging "github.com/op/go-logging"

	"github.com/gavv/emacs-jail-mcp/internal/config"
	"github.com/gavv/emacs-jail-mcp/internal/display"
)

var log = logging.MustGetLogger("container")

type Container struct {
	serverConfig *config.ServerConfig
	display      *display.Display
	running      bool
	lockFile     *os.File // held open with LOCK_EX so watchdog detects parent death
}

func New(serverConfig *config.ServerConfig, disp *display.Display) *Container {
	return &Container{serverConfig: serverConfig, display: disp}
}

func (c *Container) Start(ctx context.Context) error {
	if c.running {
		return fmt.Errorf("container %q is already running",
			c.serverConfig.ContainerName())
	}

	log.Infof("starting container %q", c.serverConfig.ContainerName())

	// Write elisp files to /tmp (shared with the container via /tmp:/tmp mount).
	if err := WriteElispFiles(c.serverConfig.ElispDir()); err != nil {
		log.Errorf("failed to write elisp files: %v", err)
		return fmt.Errorf("write elisp files: %w", err)
	}
	log.Infof("wrote elisp files to %s", c.serverConfig.ElispDir())

	// Create and hold an exclusive flock on a sentinel file in /tmp. The watchdog
	// inside the container polls this lock via "flock --nonblock". When this process
	// exits (for any reason, including SIGKILL), the OS releases the lock and the
	// watchdog can detect it without needing to see the host PID namespace.
	lockFile, err := os.OpenFile(c.serverConfig.LockPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		log.Errorf("failed to create lock file %s: %v", c.serverConfig.LockPath(), err)
		_ = os.RemoveAll(c.serverConfig.ElispDir())
		return fmt.Errorf("create lock file: %w", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		log.Errorf("failed to acquire lock %s: %v", c.serverConfig.LockPath(), err)
		_ = lockFile.Close()
		_ = os.RemoveAll(c.serverConfig.ElispDir())
		return fmt.Errorf("acquire lock: %w", err)
	}
	c.lockFile = lockFile
	log.Infof("acquired lock %s", c.serverConfig.LockPath())

	envVars := entrypointEnv(c.serverConfig, c.display)
	uid := os.Getuid()
	gid := os.Getgid()
	cwd, err := os.Getwd()
	if err != nil {
		log.Errorf("failed to get working directory: %v", err)
		_ = c.lockFile.Close()
		c.lockFile = nil
		_ = os.RemoveAll(c.serverConfig.ElispDir())
		return fmt.Errorf("get working directory: %w", err)
	}

	args := []string{
		"run",
		"--rm", "--detach", "--tty", "--init",
		"--name", c.serverConfig.ContainerName(),
		"--user", fmt.Sprintf("%d:%d", uid, gid),
		"--workdir", cwd,
		"--uts=host",
		"--network=host",
		"--ipc=host",
		"--volume", "/tmp:/tmp",
		"--volume", fmt.Sprintf("/run/user/%d:/run/user/%d", uid, uid),
		"--volume", fmt.Sprintf("%s/.cache:%s/.cache", os.Getenv("HOME"), os.Getenv("HOME")),
		"--security-opt", "label=disable",
	}
	for _, kv := range envVars {
		args = append(args, "--env", kv)
	}
	if shell := os.Getenv("SHELL"); shell != "" {
		args = append(args, "--env", "SHELL="+shell)
	}
	args = append(args,
		"--rootfs", "/:O",
		"/bin/bash", "-c", entrypointScript,
	)

	out, err := c.podman(ctx, args...)
	if err != nil {
		log.Errorf("podman run failed: %v (output: %s)", err, strings.TrimSpace(out))
		_ = c.lockFile.Close()
		c.lockFile = nil
		_ = os.RemoveAll(c.serverConfig.ElispDir())
		return fmt.Errorf("podman run: %w\noutput: %s", err, out)
	}

	c.running = true
	log.Infof("container %q started (id=%s)",
		c.serverConfig.ContainerName(), strings.TrimSpace(out))
	return nil
}

func (c *Container) Stop(ctx context.Context) error {
	if !c.running {
		return fmt.Errorf("container %q is not running",
			c.serverConfig.ContainerName())
	}

	log.Infof("stopping container %q", c.serverConfig.ContainerName())

	out, err := c.podman(ctx, "kill", c.serverConfig.ContainerName())
	if err != nil {
		log.Errorf("podman kill failed: %v (output: %s)", err, strings.TrimSpace(out))
		return fmt.Errorf("podman kill: %w\noutput: %s", err, out)
	}

	// Forcibly remove the container so its name is freed
	// immediately. This is important for restart: the same
	// container name is reused and "podman run" fails if the
	// previous container is still in "exiting" state.
	// --ignore makes the call a no-op if the container is
	// already gone.
	rmOut, rmErr := c.podman(ctx, "rm", "--force", "--ignore",
		c.serverConfig.ContainerName())
	if rmErr != nil {
		log.Warningf("podman rm failed: %v (output: %s)", rmErr, strings.TrimSpace(rmOut))
	}

	// Release the flock sentinel so the watchdog (if still
	// alive) can detect the parent has gone.  Close also
	// removes the file descriptor; the OS releases the lock
	// automatically.
	if c.lockFile != nil {
		_ = c.lockFile.Close()
		_ = os.Remove(c.serverConfig.LockPath())
		c.lockFile = nil
		log.Infof("released lock %s", c.serverConfig.LockPath())
	}

	_ = os.RemoveAll(c.serverConfig.ElispDir())

	// Remove stale Xvfb X11 socket and lock files so the
	// next start can bind to the same display number without
	// "display already in use" errors.
	_ = os.Remove(c.display.X11SocketPath())
	_ = os.Remove(c.display.X11LockPath())

	c.running = false
	log.Infof("container %q stopped", c.serverConfig.ContainerName())
	return nil
}

func (c *Container) Exec(ctx context.Context, cmd string, env ...string) (string, error) {
	log.Debugf("exec in container %q: %s", c.serverConfig.ContainerName(), cmd)

	args := []string{"exec", "--tty"}
	for _, kv := range env {
		args = append(args, "-e", kv)
	}
	args = append(args,
		c.serverConfig.ContainerName(), "/bin/bash", "-c", cmd)
	out, err := c.podman(ctx, args...)
	if err != nil {
		log.Errorf("exec failed: %v (output: %s)",
			err, strings.TrimSpace(out))
		return "", fmt.Errorf(
			"exec %q: %w\noutput: %s", cmd, err, out)
	}

	log.Debugf("exec completed (%d bytes output)", len(out))
	return out, nil
}

// Stderr is discarded.
func (c *Container) ExecRaw(
	ctx context.Context, cmd string,
) ([]byte, error) {
	log.Debugf("exec raw in container %q: %s", c.serverConfig.ContainerName(), cmd)

	args := c.podmanArgs(
		"exec", c.serverConfig.ContainerName(),
		"/bin/bash", "-c", cmd)
	log.Debugf("running: %s", strings.Join(args, " "))
	command := exec.CommandContext(ctx, args[0], args[1:]...)

	var stdout bytes.Buffer
	command.Stdout = &stdout

	if err := command.Run(); err != nil {
		log.Errorf("exec raw failed: %v", err)
		return nil, fmt.Errorf("exec raw %q: %w", cmd, err)
	}

	log.Debugf("exec raw completed (%d bytes output)", stdout.Len())
	return stdout.Bytes(), nil
}

func (c *Container) IsRunning() bool {
	return c.running
}

// If n <= 0, all lines are returned.
// The log file lives in the shared /tmp mount and is read directly from the
// host filesystem, so this works even before the container is fully started.
// Returns an empty slice if the file does not exist yet.
func (c *Container) LogLines(n int) []string {
	data, err := os.ReadFile(c.serverConfig.LogPath())
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// If n <= 0, all lines are returned.
// The file lives in the shared /tmp mount and is read directly from the
// host filesystem. Returns an empty slice if the file does not exist yet.
func (c *Container) StderrLines(n int) []string {
	data, err := os.ReadFile(c.serverConfig.StderrPath())
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func (c *Container) podman(ctx context.Context, args ...string) (string, error) {
	fullArgs := c.podmanArgs(args...)
	log.Debugf("running: %s", c.formatCommand(fullArgs))
	cmd := exec.CommandContext(ctx, fullArgs[0], fullArgs[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Debugf("command failed: %v", err)
	}
	return string(out), err
}

func (c *Container) podmanArgs(args ...string) []string {
	var full []string
	if c.serverConfig.UseSudo {
		full = append(full, "sudo")
	}
	full = append(full, c.serverConfig.PodmanBinary)
	full = append(full, args...)
	return full
}

// formatCommand returns a human-readable command line. If the last argument is a
// multi-line script (the entrypoint passed after "bash -c"), it is written to a
// temp file and the path is shown instead.
func (c *Container) formatCommand(args []string) string {
	if len(args) < 3 {
		return strings.Join(args, " ")
	}
	last := args[len(args)-1]
	if !strings.Contains(last, "\n") {
		return strings.Join(args, " ")
	}
	name := fmt.Sprintf("/tmp/%s-entrypoint.sh", c.serverConfig.JailID())
	if err := os.WriteFile(name, []byte(last), 0o644); err != nil {
		return strings.Join(args[:len(args)-1], " ") + " <script>"
	}
	return strings.Join(args[:len(args)-1], " ") + " " + name
}

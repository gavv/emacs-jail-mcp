// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package config

import (
	"fmt"
	"os"
	"time"
)

// ServerConfig holds all configuration for the server subcommand.
// It is consumed by the jail and container layers.
type ServerConfig struct {
	PodmanBinary    string
	ContainerPrefix string
	UseSudo         bool
	Display         DisplayConfig

	EmacsBinary    string
	EmacsSocketDir string

	MCPHost string
	MCPPort int

	StartTimeout time.Duration
	StopTimeout  time.Duration
	ExecTimeout  time.Duration
}

func DefaultServer() *ServerConfig {
	return &ServerConfig{
		PodmanBinary:    "podman",
		ContainerPrefix: "emacs-jail",
		UseSudo:         true,
		Display: DisplayConfig{
			Depth: DefaultDisplayDepth,
		},

		EmacsBinary:    "emacs",
		EmacsSocketDir: "/tmp",

		MCPHost: "127.0.0.1",
		MCPPort: DefaultMCPPort,

		StartTimeout: 240 * time.Second,
		StopTimeout:  10 * time.Second,
		ExecTimeout:  30 * time.Second,
	}
}

func (c *ServerConfig) JailID() string {
	return fmt.Sprintf("%s-%d", c.ContainerPrefix, os.Getpid())
}

func (c *ServerConfig) ContainerName() string {
	return c.JailID()
}

func (c *ServerConfig) SocketPath() string {
	return fmt.Sprintf("%s/%s.sock", c.EmacsSocketDir, c.JailID())
}

// MCPAddr returns the TCP address (host:port) for the MCP SSE server.
func (c *ServerConfig) MCPAddr() string {
	return fmt.Sprintf("%s:%d", c.MCPHost, c.MCPPort)
}

// ElispDir returns the directory where emacs-jail-rpc elisp files are written
// before the container starts. The directory is inside EmacsSocketDir so that
// it is accessible inside the container via the /tmp:/tmp volume mount.
func (c *ServerConfig) ElispDir() string {
	return fmt.Sprintf("%s/%s-elisp", c.EmacsSocketDir, c.JailID())
}

func (c *ServerConfig) LogPath() string {
	return fmt.Sprintf("%s/%s.log", c.EmacsSocketDir, c.JailID())
}

func (c *ServerConfig) StderrPath() string {
	return fmt.Sprintf("%s/%s.stderr", c.EmacsSocketDir, c.JailID())
}

// LockPath returns the path to the flock sentinel file used by the watchdog. The Go
// process holds an exclusive flock on this file; the watchdog inside the container polls
// it with "flock --nonblock". When the Go process exits (for any reason, including
// SIGKILL), the OS releases the lock and the watchdog can detect parent death without
// needing the host PID namespace.
func (c *ServerConfig) LockPath() string {
	return fmt.Sprintf("%s/%s.lock", c.EmacsSocketDir, c.JailID())
}

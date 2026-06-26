// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package config

import (
	"fmt"
	"time"
)

// ClientConfig holds configuration for the client and info subcommands.
type ClientConfig struct {
	MCPHost string
	MCPPort int

	StartTimeout time.Duration
	ExecTimeout  time.Duration
}

func DefaultClient() *ClientConfig {
	return &ClientConfig{
		MCPHost: "127.0.0.1",
		MCPPort: DefaultMCPPort,

		StartTimeout: 240 * time.Second,
		ExecTimeout:  30 * time.Second,
	}
}

// MCPAddr returns the TCP address (host:port) for the MCP SSE server.
func (c *ClientConfig) MCPAddr() string {
	return fmt.Sprintf("%s:%d", c.MCPHost, c.MCPPort)
}

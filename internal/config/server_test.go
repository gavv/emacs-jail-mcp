// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gavv/emacs-jail-mcp/internal/config"
)

func TestServerDefaultValues(t *testing.T) {
	cfg := config.DefaultServer()

	assert.Equal(t, "podman", cfg.PodmanBinary)
	assert.True(t, cfg.UseSudo)
	assert.Equal(t, "127.0.0.1", cfg.MCPHost)
	assert.Equal(t, config.DefaultMCPPort, cfg.MCPPort)
	assert.Equal(t, "emacs", cfg.EmacsBinary)
}

func TestServerMCPAddr(t *testing.T) {
	cfg := config.DefaultServer()
	cfg.MCPHost = "0.0.0.0"
	cfg.MCPPort = 1234

	assert.Equal(t, "0.0.0.0:1234", cfg.MCPAddr())
}

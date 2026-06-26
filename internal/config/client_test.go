// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gavv/emacs-jail-mcp/internal/config"
)

func TestClientDefaultValues(t *testing.T) {
	cfg := config.DefaultClient()

	assert.Equal(t, "127.0.0.1", cfg.MCPHost)
	assert.Equal(t, config.DefaultMCPPort, cfg.MCPPort)
}

func TestClientMCPAddr(t *testing.T) {
	cfg := config.DefaultClient()
	cfg.MCPHost = "10.0.0.1"
	cfg.MCPPort = 5678

	assert.Equal(t, "10.0.0.1:5678", cfg.MCPAddr())
}

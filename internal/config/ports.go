// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package config

const (
	// DefaultMCPPort is the default TCP port for the MCP SSE server.
	DefaultMCPPort = 9421

	// E2ETestMCPPort is the port used by the e2e test suite to avoid
	// conflicting with a production server running on DefaultMCPPort.
	E2ETestMCPPort = 9422
)

// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package container

import (
	_ "embed"
	"fmt"

	"github.com/gavv/emacs-jail-mcp/internal/config"
	"github.com/gavv/emacs-jail-mcp/internal/display"
)

//go:embed entrypoint.sh
var entrypointScript string

// entrypointEnv returns the environment variables that the entrypoint script reads to
// configure itself. They are passed via --env flags to podman run.
func entrypointEnv(cfg *config.ServerConfig, disp *display.Display) []string {
	return []string{
		fmt.Sprintf("EMACS_JAIL_LOCK_PATH=%s", cfg.LockPath()),
		fmt.Sprintf("EMACS_JAIL_SOCKET_PATH=%s", cfg.SocketPath()),
		fmt.Sprintf("EMACS_JAIL_LOG_PATH=%s", cfg.LogPath()),
		fmt.Sprintf("EMACS_JAIL_DISPLAY=%s", disp.String()),
		fmt.Sprintf("EMACS_JAIL_DISPLAY_NUMBER=%d", disp.Number),
		fmt.Sprintf("EMACS_JAIL_DISPLAY_WIDTH=%d", disp.Width),
		fmt.Sprintf("EMACS_JAIL_DISPLAY_HEIGHT=%d", disp.Height),
		fmt.Sprintf("EMACS_JAIL_DISPLAY_DEPTH=%d", disp.Depth),
		fmt.Sprintf("EMACS_JAIL_ELISP_DIR=%s", cfg.ElispDir()),
		fmt.Sprintf("EMACS_JAIL_EMACS_BINARY=%s", cfg.EmacsBinary),
		fmt.Sprintf("EMACS_JAIL_STDERR_PATH=%s", cfg.StderrPath()),
	}
}

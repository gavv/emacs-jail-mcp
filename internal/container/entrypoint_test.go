// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package container

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gavv/emacs-jail-mcp/internal/config"
	"github.com/gavv/emacs-jail-mcp/internal/display"
)

func TestEntrypointEnvCoversAllVars(t *testing.T) {
	cfg := config.DefaultServer()
	env := entrypointEnv(cfg, display.Default(20))

	// Build a set of env var names from the returned slice.
	envNames := make(map[string]bool, len(env))
	for _, kv := range env {
		name, _, ok := strings.Cut(kv, "=")
		assert.True(t, ok, "malformed env entry (no '='): %q", kv)
		if ok {
			envNames[name] = true
		}
	}

	// Every EMACS_JAIL_* variable referenced in the script must be provided.
	for _, varName := range jailVarsInScript() {
		assert.True(t, envNames[varName],
			"script references $%s but entrypointEnv does not provide it", varName)
	}

	// Every variable provided must be referenced in the script.
	for name := range envNames {
		referenced := strings.Contains(entrypointScript, "$"+name) ||
			strings.Contains(entrypointScript, "${"+name+"}")
		assert.True(t, referenced,
			"entrypointEnv provides %s but script does not reference it", name)
	}
}

func TestEntrypointStartsEmbeddedRPCServer(t *testing.T) {
	assert.Contains(t, entrypointScript, `(require (quote emacs-jail-rpc))`)
	assert.Contains(t, entrypointScript, `(emacs-jail-rpc-start \"${EMACS_JAIL_SOCKET_PATH}\")`)
	assert.NotContains(t, entrypointScript, `emacs-jail-start`)
}

// jailVarsInScript extracts all EMACS_JAIL_* variable names referenced in the
// embedded entrypoint script (both $VAR and ${VAR} forms).
func jailVarsInScript() []string {
	var vars []string
	seen := make(map[string]bool)

	rest := entrypointScript
	for {
		// Find the next occurrence of $EMACS_JAIL_ or ${EMACS_JAIL_
		idx := strings.Index(rest, "$EMACS_JAIL_")
		if idx == -1 {
			break
		}
		rest = rest[idx+1:] // skip '$'

		var name string
		if strings.HasPrefix(rest, "{") {
			// ${VAR} form — read until '}'
			end := strings.IndexByte(rest, '}')
			if end == -1 {
				break
			}
			name = rest[1:end]
		} else {
			// $VAR form — read until non-identifier character
			end := strings.IndexAny(rest, " \t\n\r\"'\\/${}();")
			if end == -1 {
				name = rest
			} else {
				name = rest[:end]
			}
		}

		if !seen[name] {
			seen[name] = true
			vars = append(vars, name)
		}
	}
	return vars
}

// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package display_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gavv/emacs-jail-mcp/internal/display"
)

// TestNumberRange verifies display numbers are always in [20, 999].
func TestNumberRange(t *testing.T) {
	for _, pid := range []int{0, 1, 979, 980, 12345} {
		n := display.Number(pid)
		assert.GreaterOrEqual(t, n, 20)
		assert.LessOrEqual(t, n, 999)
	}
}

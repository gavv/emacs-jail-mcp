// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package config

const (
	DefaultDisplayWidth  = 1280
	DefaultDisplayHeight = 1024
	DefaultDisplayDepth  = 24
)

// DisplayConfig holds user-provided display overrides from CLI flags.
// Zero width or height means auto-detect from the host display, then fall back
// to default dimensions.
type DisplayConfig struct {
	Width  int
	Height int
	Depth  int
}

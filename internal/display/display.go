// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package display

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/gavv/emacs-jail-mcp/internal/config"
)

var xdpyinfoRe = regexp.MustCompile(`dimensions:\s+(\d+)x(\d+)\s+pixels`)

type Display struct {
	Number int
	Width  int
	Height int
	Depth  int
}

func New(cfg config.DisplayConfig) *Display {
	display := Default(Number(os.Getpid()))
	if w, h := HostSize(); w != 0 && h != 0 {
		display.Width = w
		display.Height = h
	}
	if cfg.Width != 0 {
		display.Width = cfg.Width
	}
	if cfg.Height != 0 {
		display.Height = cfg.Height
	}
	if cfg.Depth != 0 {
		display.Depth = cfg.Depth
	}
	return display
}

func Default(number int) *Display {
	return &Display{
		Number: number,
		Width:  config.DefaultDisplayWidth,
		Height: config.DefaultDisplayHeight,
		Depth:  config.DefaultDisplayDepth,
	}
}

func Number(pid int) int {
	return 20 + (pid % 980)
}

func (d *Display) String() string {
	return fmt.Sprintf(":%d", d.Number)
}

func (d *Display) X11SocketPath() string {
	return fmt.Sprintf("/tmp/.X11-unix/X%d", d.Number)
}

func (d *Display) X11LockPath() string {
	return fmt.Sprintf("/tmp/.X%d-lock", d.Number)
}

// HostSize queries the host X display for its pixel dimensions.
// It tries $DISPLAY first, then falls back to :0.
// Returns (0, 0) if the display cannot be queried.
func HostSize() (width, height int) {
	displays := []string{os.Getenv("DISPLAY"), ":0"}
	for _, d := range displays {
		if d == "" {
			continue
		}
		out, err := exec.Command("xdpyinfo", "-display", d).Output()
		if err != nil {
			continue
		}
		m := xdpyinfoRe.FindSubmatch(out)
		if m == nil {
			continue
		}
		w, err1 := strconv.Atoi(string(m[1]))
		h, err2 := strconv.Atoi(string(m[2]))
		if err1 != nil || err2 != nil {
			continue
		}
		return w, h
	}
	return 0, 0
}

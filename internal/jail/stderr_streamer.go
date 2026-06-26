// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package jail

import (
	"bufio"
	"context"
	"os/exec"
)

// stderrStreamer runs "tail -f <path>" and calls onLine for each new line.
// It runs until stop() is called.
type stderrStreamer struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// startStderrStreamer starts a goroutine that runs
// "tail -n +1 --retry --follow=name path", calling onLine for each new
// complete line of output. The goroutine runs until stop() is called.
// Returns immediately; tailing runs in the background.
//
// Flags used:
//   - "-n +1": emit all existing content from the start of the file before
//     following new writes, so lines written before the streamer starts are
//     not missed.
//   - "--retry --follow=name": keep retrying if the file does not exist yet
//     or is replaced (e.g. on jail restart), so the streamer is robust to
//     the file appearing after start is called.
func startStderrStreamer(
	ctx context.Context,
	path string,
	onLine func(string),
) *stderrStreamer {
	ctx, cancel := context.WithCancel(ctx)
	s := &stderrStreamer{
		cancel: cancel,
		done:   make(chan struct{}),
	}
	go s.run(ctx, path, onLine)
	return s
}

func (s *stderrStreamer) stop() {
	s.cancel()
	<-s.done
}

func (s *stderrStreamer) run(ctx context.Context, path string, onLine func(string)) {
	defer close(s.done)

	cmd := exec.CommandContext(ctx, "tail", "-n", "+1", "--retry", "--follow=name", path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		onLine(scanner.Text())
	}

	_ = cmd.Wait()
}

// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package jail

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStderrStreamer(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stderr-*.txt")
	require.NoError(t, err)
	path := f.Name()

	var received []string
	s := startStderrStreamer(context.Background(), path, func(line string) {
		received = append(received, line)
	})
	defer s.stop()

	_, err = f.WriteString("hello\nworld\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	require.Eventually(t, func() bool {
		return len(received) >= 2
	}, 5*time.Second, 50*time.Millisecond)

	assert.Equal(t, []string{"hello", "world"}, received[:2])
}

func TestStderrStreamerPartialLines(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stderr-*.txt")
	require.NoError(t, err)
	path := f.Name()

	var received []string
	s := startStderrStreamer(context.Background(), path, func(line string) {
		received = append(received, line)
	})
	defer s.stop()

	// Write without a trailing newline — tail -f won't deliver until newline.
	_, err = f.WriteString("partial")
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)
	assert.Empty(t, received, "partial line must not be delivered before newline")

	// Complete the line.
	_, err = f.WriteString(" line\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	require.Eventually(t, func() bool {
		return len(received) >= 1
	}, 5*time.Second, 50*time.Millisecond)

	assert.Equal(t, "partial line", received[0])
}

func TestStderrStreamerMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notyet.txt")

	// File does not exist — tail --retry --follow=name will wait for it.
	var received []string
	s := startStderrStreamer(context.Background(), path, func(line string) {
		received = append(received, line)
	})
	defer s.stop()

	time.Sleep(100 * time.Millisecond)
	assert.Empty(t, received)

	// Create and write to it.
	f, err := os.Create(path)
	require.NoError(t, err)
	_, err = f.WriteString("appeared\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	require.Eventually(t, func() bool {
		return len(received) >= 1
	}, 5*time.Second, 50*time.Millisecond)

	assert.Equal(t, "appeared", received[0])
}

func TestStderrStreamerStop(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stderr-*.txt")
	require.NoError(t, err)
	path := f.Name()

	var received []string
	s := startStderrStreamer(context.Background(), path, func(line string) {
		received = append(received, line)
	})

	_, err = f.WriteString("before-stop\n")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(received) >= 1
	}, 5*time.Second, 50*time.Millisecond)

	// stop() must return promptly.
	done := make(chan struct{})
	go func() {
		s.stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stop() did not return within 3 seconds")
	}

	snapshot := len(received)

	_, err = f.WriteString("after-stop\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	time.Sleep(200 * time.Millisecond)

	assert.Equal(t, snapshot, len(received),
		"no new lines should arrive after stop")
}

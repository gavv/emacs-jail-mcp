// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

//go:build e2e

package e2e_test

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/op/go-logging"
)

const (
	logFmt = `%{time:15:04:05.000} %{level:.4s} [%{module}] %{message}`
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestMain(m *testing.M) {
	var logBuf syncBuffer
	backend := logging.NewBackendFormatter(
		logging.NewLogBackend(&logBuf, "", 0),
		logging.MustStringFormatter(logFmt),
	)
	logging.SetBackend(backend)

	code := m.Run()

	if code != 0 {
		fmt.Fprintf(os.Stderr,
			"Server logs:\n%s", logBuf.String())
	}

	os.Exit(code)
}

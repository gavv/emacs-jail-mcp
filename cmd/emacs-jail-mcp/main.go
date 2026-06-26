// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package main

import (
	"os"

	"github.com/op/go-logging"

	"github.com/gavv/emacs-jail-mcp/internal/cli"
)

var log = logging.MustGetLogger("main")

const (
	logFmt = `%{color}%{time:15:04:05.000} %{level:.4s} [%{module}] %{message}%{color:reset}`
)

func main() {
	logging.SetBackend(
		logging.NewBackendFormatter(
			logging.NewLogBackend(os.Stderr, "", 0),
			logging.MustStringFormatter(logFmt),
		))

	if err := cli.NewRootCmd().Execute(); err != nil {
		log.Errorf("%v", err)
		os.Exit(1)
	}
}

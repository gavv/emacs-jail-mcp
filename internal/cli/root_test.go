// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpPreservesServeFlagOrder(t *testing.T) {
	out := runHelp(t, "serve")
	assertBefore(t, out, "--stdio", "--podman-binary")
	assertBefore(t, out, "--podman-binary", "--no-sudo")
	assertBefore(t, out, "--display-depth", "--emacs-binary")
	assertBefore(t, out, "--mcp-port", "--start-timeout")
}

func TestHelpPreservesInheritedSendFlagOrder(t *testing.T) {
	out := runHelp(t, "send", "eval")
	assertBefore(t, out, "--mcp-host", "--mcp-port")
	assertBefore(t, out, "--mcp-port", "--start-timeout")
	assertBefore(t, out, "--start-timeout", "--exec-timeout")
}

func TestManualIncludesDefaultHelpAndCompletionCommands(t *testing.T) {
	cmd := NewRootCmd()
	buf := bytes.NewBuffer(nil)

	if err := writeManual(buf, cmd, manualFormatMarkdown); err != nil {
		t.Fatalf("writeManual() error = %v", err)
	}
	out := buf.String()

	assertContains(t, out, "emacs-jail-mcp completion")
	assertContains(t, out, "emacs-jail-mcp help")
}

func runHelp(t *testing.T, args ...string) string {
	t.Helper()

	cmd := NewRootCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(append(args, "--help"))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	return buf.String()
}

func assertContains(t *testing.T, output, needle string) {
	t.Helper()

	if !strings.Contains(output, needle) {
		t.Fatalf("%q not found in output:\n%s", needle, output)
	}
}

func assertBefore(t *testing.T, output, first, second string) {
	t.Helper()

	firstPos := strings.Index(output, first)
	if firstPos < 0 {
		t.Fatalf("%q not found in help output:\n%s", first, output)
	}
	secondPos := strings.Index(output, second)
	if secondPos < 0 {
		t.Fatalf("%q not found in help output:\n%s", second, output)
	}
	if firstPos > secondPos {
		t.Fatalf("expected %q before %q in help output:\n%s", first, second, output)
	}
}

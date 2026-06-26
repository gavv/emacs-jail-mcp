// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package cli

import (
	"fmt"
	"io"

	"github.com/gavv/cobradoc"
	"github.com/spf13/cobra"
)

const (
	manualFormatTroff    = "troff"
	manualFormatMarkdown = "markdown"
)

func newManCmd() *cobra.Command {
	format := manualFormatTroff
	cmd := &cobra.Command{
		Use:   "man",
		Short: "Generate manual page",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeManual(cmd.OutOrStdout(), cmd.Root(), format)
		},
	}

	cmd.Flags().StringVar(&format, "format", format, "output format: troff or markdown")
	return cmd
}

func writeManual(w io.Writer, rootCmd *cobra.Command, format string) error {
	docFormat, err := parseManualFormat(format)
	if err != nil {
		return err
	}

	return cobradoc.WriteDocument(w, rootCmd, docFormat, cobradoc.Options{
		Name:             "emacs-jail-mcp",
		Header:           "Emacs Jail MCP Manual",
		Footer:           "Emacs Jail MCP Manual",
		ShortDescription: "MCP server for running Emacs in a disposable jail",
	})
}

func parseManualFormat(format string) (cobradoc.Format, error) {
	switch format {
	case manualFormatTroff:
		return cobradoc.Troff, nil
	case manualFormatMarkdown:
		return cobradoc.Markdown, nil
	default:
		return 0, fmt.Errorf("unknown manual format %q", format)
	}
}

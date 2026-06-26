// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd builds and returns the root cobra command.
func NewRootCmd() *cobra.Command {
	cobra.EnableCommandSorting = false

	cmd := &cobra.Command{
		Use:   "emacs-jail-mcp",
		Short: "MCP server that runs Emacs inside a disposable Podman container",
		//nolint
		Long: `Emacs Jail MCP

Manages an Emacs instance running inside a copy-on-write disposable sandbox. Allows LLMs to control sandbox container, run elisp code, and capture logs and screenshots.`,

		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newServerCmd())
	cmd.AddCommand(newInfoCmd())
	cmd.AddCommand(newClientCmd())
	cmd.AddCommand(newManCmd())

	cmd.InitDefaultHelpCmd()
	cmd.InitDefaultCompletionCmd()

	disableFlagSorting(cmd)
	configureHelpFlagSorting(cmd)

	return cmd
}

func configureHelpFlagSorting(cmd *cobra.Command) {
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		disableFlagSorting(cmd.Root())
		cmd.Println(cmd.UsageString())
	})

	for _, child := range cmd.Commands() {
		configureHelpFlagSorting(child)
	}
}

func disableFlagSorting(cmd *cobra.Command) {
	cmd.PersistentFlags().SortFlags = false
	cmd.Flags().SortFlags = false
	cmd.NonInheritedFlags().SortFlags = false
	cmd.LocalFlags().SortFlags = false
	cmd.InheritedFlags().SortFlags = false

	for _, child := range cmd.Commands() {
		disableFlagSorting(child)
	}
}

// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package cli

import (
	"fmt"
	"net"
	"time"

	"github.com/spf13/cobra"

	"github.com/gavv/emacs-jail-mcp/internal/config"
)

func newInfoCmd() *cobra.Command {
	cfg := config.DefaultClient()

	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show MCP server address and whether it is running",
		Long:  `Check whether the MCP SSE server is listening on the configured TCP address.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Emacs Jail MCP server\n\n")

			addr := cfg.MCPAddr()
			fmt.Printf("Address:  http://%s/sse\n", addr)

			conn, err := net.DialTimeout(
				"tcp", addr, time.Second,
			)
			if err != nil {
				fmt.Printf("Status:   offline\n")
				return nil
			}
			_ = conn.Close()
			fmt.Println("Status:   online")
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	pfs := cmd.Flags()
	pfs.SortFlags = false
	pfs.StringVarP(&cfg.MCPHost,
		"mcp-host", "H", cfg.MCPHost,
		"host of the MCP SSE server to connect to")
	pfs.IntVarP(&cfg.MCPPort,
		"mcp-port", "P", cfg.MCPPort,
		"TCP port of the MCP SSE server")

	return cmd
}

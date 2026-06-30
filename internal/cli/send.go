// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	logging "github.com/op/go-logging"
	"github.com/spf13/cobra"

	"github.com/gavv/emacs-jail-mcp/internal/config"
)

var clientLog = logging.MustGetLogger("client")

func newClientCmd() *cobra.Command {
	cfg := config.DefaultClient()

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send MCP requests from the command line",
		//nolint
		Long: `Send MCP requests from the command line.

Each subcommand corresponds to one MCP tool and connects to an already-running server via
its TCP port (SSE transport).`,

		SilenceUsage:  true,
		SilenceErrors: true,
	}

	pfs := cmd.PersistentFlags()
	pfs.SortFlags = false
	pfs.StringVarP(&cfg.MCPHost, "mcp-host", "H", cfg.MCPHost,
		"host of the MCP SSE server to connect to")
	pfs.IntVarP(&cfg.MCPPort, "mcp-port", "P", cfg.MCPPort,
		"TCP port of the MCP SSE server")
	pfs.DurationVar(&cfg.StartTimeout,
		"start-timeout", cfg.StartTimeout, "timeout for jail start/stop operations")
	pfs.DurationVar(&cfg.ExecTimeout,
		"exec-timeout", cfg.ExecTimeout,
		"timeout for eval, logs, bytecomp, and shell operations")

	subCmds := []*cobra.Command{
		newControlCmd(cfg),
		newEvalCmd(cfg),
		newBytecompCmd(cfg),
		newShellCmd(cfg),
		newScreenshotCmd(cfg),
		newLogsCmd(cfg),
	}
	for _, sub := range subCmds {
		cmd.AddCommand(sub)
		sub.InheritedFlags().SortFlags = false
	}

	return cmd
}

// withMCPClient connects to the MCP server over TCP, initializes the protocol,
// calls fn, and cleans up.
func withMCPClient(
	cfg *config.ClientConfig,
	timeout time.Duration,
	fn func(ctx context.Context, c *mcpclient.Client) error,
) error {
	addr := cfg.MCPAddr()
	baseURL := fmt.Sprintf("http://%s/sse", addr)

	clientLog.Infof("connecting to MCP server at %s", addr)

	c, err := mcpclient.NewSSEMCPClient(baseURL)
	if err != nil {
		clientLog.Errorf("failed to create MCP client: %v", err)
		return fmt.Errorf("create MCP client: %w", err)
	}
	defer func() {
		clientLog.Infof("closing MCP client connection")
		_ = c.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		clientLog.Errorf("failed to connect to MCP server at %s: %v", addr, err)
		return fmt.Errorf("connect to MCP server at %s: %w", addr, err)
	}

	clientLog.Infof("connected to MCP server at %s", addr)
	clientLog.Infof("initializing MCP session")

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "emacs-jail-mcp-cli",
		Version: "0.1.0",
	}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		clientLog.Errorf("failed to initialize MCP session: %v", err)
		return fmt.Errorf("initialize MCP session: %w", err)
	}

	clientLog.Infof("MCP session initialized")

	return fn(ctx, c)
}

// callToolText calls a tool and returns the first text content.
func callToolText(
	ctx context.Context,
	c *mcpclient.Client,
	name string,
	args map[string]any,
) (string, error) {
	clientLog.Infof("calling tool %q", name)

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := c.CallTool(ctx, req)
	if err != nil {
		clientLog.Errorf("tool %q call failed: %v", name, err)
		return "", err
	}
	if result.IsError {
		for _, c := range result.Content {
			if tc, ok := c.(mcp.TextContent); ok {
				clientLog.Errorf("tool %q returned error: %s", name, tc.Text)
				return "", errors.New(tc.Text)
			}
		}
		clientLog.Errorf("tool %q returned error", name)
		return "", errors.New("tool returned error")
	}
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			clientLog.Infof("tool %q succeeded", name)
			return tc.Text, nil
		}
	}
	clientLog.Infof("tool %q returned empty result", name)
	return "", nil
}

func newControlCmd(cfg *config.ClientConfig) *cobra.Command {
	var start, stop, restart, status bool

	cmd := &cobra.Command{
		Use:     "control",
		Aliases: []string{"ctl"},
		Short:   "Control the jail lifecycle",
		//nolint
		Long: `Control the jail lifecycle.

Specify exactly one action flag: --start, --stop, --restart, or --status.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var action string
			switch {
			case start:
				action = "start"
			case stop:
				action = "stop"
			case restart:
				action = "restart"
			case status:
				action = "status"
			}
			timeout := cfg.StartTimeout
			return withMCPClient(cfg, timeout,
				func(ctx context.Context, c *mcpclient.Client) error {
					text, err := callToolText(ctx, c, "control",
						map[string]any{"action": action},
					)
					if err != nil {
						return err
					}
					fmt.Println(text)
					return nil
				},
			)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	fs := cmd.Flags()
	fs.SortFlags = false
	fs.BoolVar(&start, "start", false, "start the jail")
	fs.BoolVar(&stop, "stop", false, "stop the jail")
	fs.BoolVar(&restart, "restart", false, "restart the jail")
	fs.BoolVar(&status, "status", false, "show jail status")

	cmd.MarkFlagsOneRequired("start", "stop", "restart", "status")
	cmd.MarkFlagsMutuallyExclusive("start", "stop", "restart", "status")

	return cmd
}

func newEvalCmd(cfg *config.ClientConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "eval EXPRESSION",
		Aliases: []string{"ev"},
		Short:   "Evaluate an Emacs Lisp expression",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMCPClient(cfg, cfg.ExecTimeout,
				func(ctx context.Context, c *mcpclient.Client) error {
					text, err := callToolText(ctx, c, "eval",
						map[string]any{
							"expression": args[0],
						},
					)
					if err != nil {
						return err
					}
					fmt.Println(text)
					return nil
				},
			)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	return cmd
}

func newLogsCmd(cfg *config.ClientConfig) *cobra.Command {
	var sources string
	var offset int64
	var limit int64

	cmd := &cobra.Command{
		Use:     "logs",
		Aliases: []string{"lg"},
		Short:   "Fetch Emacs log and diagnostic output",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMCPClient(cfg, cfg.ExecTimeout,
				func(ctx context.Context, c *mcpclient.Client) error {
					toolArgs := map[string]any{}
					if sources != "" {
						toolArgs["sources"] = sources
					}
					if offset != 0 {
						toolArgs["offset"] = offset
					}
					if limit != 0 {
						toolArgs["limit"] = limit
					}
					text, err := callToolText(ctx, c, "logs", toolArgs)
					if err != nil {
						return err
					}
					fmt.Print(text)
					return nil
				},
			)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	fs := cmd.Flags()
	fs.SortFlags = false
	fs.StringVarP(&sources, "sources", "s", "",
		"comma-separated log sources "+
			"(messages, warnings, backtrace, compile_log, async_compile_log, "+
			"init_log, stderr)")
	fs.Int64VarP(&offset, "offset", "o", 0,
		"1-based line offset within each source section")
	fs.Int64VarP(&limit, "limit", "l", 0, "max lines to return per source section")

	return cmd
}

func newShellCmd(cfg *config.ClientConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "shell COMMAND",
		Aliases: []string{"sh"},
		Short:   "Run a shell command inside the jail container",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMCPClient(cfg, cfg.ExecTimeout,
				func(ctx context.Context, c *mcpclient.Client) error {
					text, err := callToolText(ctx, c, "shell",
						map[string]any{"command": args[0]},
					)
					if err != nil {
						return err
					}
					fmt.Print(text)
					return nil
				},
			)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	return cmd
}

func newScreenshotCmd(cfg *config.ClientConfig) *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:     "screenshot",
		Aliases: []string{"sc"},
		Short:   "Capture a screenshot of the Emacs display",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMCPClient(cfg, 30*time.Second,
				func(ctx context.Context, c *mcpclient.Client) error {
					clientLog.Infof("calling tool %q", "screenshot")

					req := mcp.CallToolRequest{}
					req.Params.Name = "screenshot"

					result, err := c.CallTool(ctx, req)
					if err != nil {
						clientLog.Errorf("tool %q call failed: %v", "screenshot", err)
						return err
					}
					if result.IsError {
						for _, ct := range result.Content {
							if tc, ok := ct.(mcp.TextContent); ok {
								clientLog.Errorf("tool %q returned error: %s",
									"screenshot", tc.Text)
								return errors.New(tc.Text)
							}
						}
						return errors.New("screenshot failed")
					}
					for _, ct := range result.Content {
						if ic, ok := ct.(mcp.ImageContent); ok {
							clientLog.Debugf("writing screenshot to %s", outputPath)
							return writeBase64PNG(ic.Data, outputPath)
						}
					}
					return errors.New("no image in response")
				},
			)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	fs := cmd.Flags()
	fs.SortFlags = false
	fs.StringVarP(&outputPath, "output", "o", "", "output file path for the PNG screenshot")
	_ = cmd.MarkFlagRequired("output")

	return cmd
}

func newBytecompCmd(cfg *config.ClientConfig) *cobra.Command {
	var filePath string
	var severity string

	cmd := &cobra.Command{
		Use:     "bytecomp",
		Aliases: []string{"bc"},
		Short:   "Byte-compile an Elisp file and return diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			if severity != "" && severity != "error" && severity != "warning" {
				return errors.New("--severity must be 'error' or 'warning'")
			}

			return withMCPClient(cfg, cfg.ExecTimeout,
				func(ctx context.Context, c *mcpclient.Client) error {
					toolArgs := map[string]any{
						"file_path": filePath,
					}
					if severity != "" {
						toolArgs["severity"] = severity
					}
					text, err := callToolText(ctx, c, "bytecomp", toolArgs)
					if err != nil {
						return err
					}
					fmt.Println(text)
					return nil
				},
			)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	fs := cmd.Flags()
	fs.SortFlags = false
	fs.StringVarP(&filePath, "file-path", "f", "",
		"path to the Elisp file to byte-compile")
	_ = cmd.MarkFlagRequired("file-path")
	fs.StringVarP(&severity, "severity", "s", "", "filter by severity: error or warning")

	return cmd
}

func writeBase64PNG(b64, outputPath string) error {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("decode image: %w", err)
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outputPath, err)
	}
	return nil
}

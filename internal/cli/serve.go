// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
	logging "github.com/op/go-logging"
	"github.com/spf13/cobra"

	"github.com/gavv/emacs-jail-mcp/internal/config"
	"github.com/gavv/emacs-jail-mcp/internal/jail"
	"github.com/gavv/emacs-jail-mcp/internal/tools"
)

var serverLog = logging.MustGetLogger("server")

func newServerCmd() *cobra.Command {
	cfg := config.DefaultServer()
	var stdio bool
	var noSudo bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the MCP server",
		//nolint
		Long: `Run the MCP server.

Without --stdio (default): listens on a TCP port (SSE transport) for MCP client and
CLI client connections.

With --stdio: reads MCP JSON-RPC messages from stdin/stdout (stdio transport).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.UseSudo = !noSudo
			return runServer(cfg, stdio)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	pfs := cmd.Flags()
	pfs.SortFlags = false

	pfs.BoolVar(&stdio,
		"stdio", false, "use stdio transport instead of TCP SSE transport")

	pfs.StringVar(&cfg.PodmanBinary,
		"podman-binary", cfg.PodmanBinary, "podman binary name or path")
	pfs.BoolVar(&noSudo,
		"no-sudo", false, "disable sudo for podman commands")
	pfs.IntVar(&cfg.Display.Width,
		"display-width", cfg.Display.Width,
		"override auto-detected Xvfb display width in pixels")
	pfs.IntVar(&cfg.Display.Height,
		"display-height", cfg.Display.Height,
		"override auto-detected Xvfb display height in pixels")
	pfs.IntVar(&cfg.Display.Depth,
		"display-depth", cfg.Display.Depth,
		"Xvfb display color depth")

	pfs.StringVarP(&cfg.EmacsBinary,
		"emacs-binary", "e", cfg.EmacsBinary, "emacs binary name or path")
	pfs.StringVar(&cfg.EmacsSocketDir,
		"emacs-socket-dir", cfg.EmacsSocketDir,
		"directory for the emacs-jail-rpc Unix socket")

	pfs.StringVarP(&cfg.MCPHost,
		"mcp-host", "H", cfg.MCPHost,
		"host for the MCP SSE server to bind to")
	pfs.IntVarP(&cfg.MCPPort,
		"mcp-port", "P", cfg.MCPPort,
		"TCP port for the MCP SSE server")

	pfs.DurationVar(&cfg.StartTimeout,
		"start-timeout", cfg.StartTimeout, "timeout waiting for jail to start")
	pfs.DurationVar(&cfg.StopTimeout,
		"stop-timeout", cfg.StopTimeout, "timeout waiting for jail to stop")
	pfs.DurationVar(&cfg.ExecTimeout,
		"exec-timeout", cfg.ExecTimeout, "timeout for elisp eval and shell commands")

	return cmd
}

func runServer(cfg *config.ServerConfig, stdio bool) error {
	serverLog.Infof("starting MCP server (jail=%s)", cfg.JailID())

	sb := jail.New(cfg)
	mcpSrv := server.NewMCPServer(
		"emacs-jail-mcp", "0.1.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)
	tools.Register(mcpSrv, sb)

	var ln net.Listener
	var httpSrv *http.Server

	if !stdio {
		var err error
		ln, httpSrv, err = startSSEListener(mcpSrv, cfg)
		if err != nil {
			serverLog.Errorf(
				"failed to start SSE listener: %v", err)
			return err
		}
	}

	cleanup := func() {
		if httpSrv != nil {
			serverLog.Infof("shutting down HTTP server")
			shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutCancel()
			_ = httpSrv.Shutdown(shutCtx)
		}
		if ln != nil {
			_ = ln.Close()
		}
		if sb.IsRunning() {
			serverLog.Infof("stopping jail during cleanup")
			_ = sb.Stop(context.Background())
		}
	}
	defer cleanup()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		serverLog.Infof("received signal %s, shutting down", sig)
		cleanup()
		os.Exit(0)
	}()

	if stdio {
		serverLog.Infof("serving MCP stdio transport")
		if err := server.ServeStdio(mcpSrv); err != nil {
			serverLog.Errorf("MCP stdio transport error: %v", err)
			return fmt.Errorf("serve: %w", err)
		}
		serverLog.Infof("MCP stdio transport closed")
	} else {
		stdin := os.Stdin
		stdinCh := make(chan struct{})
		go func() {
			buf := make([]byte, 1)
			for {
				_, err := stdin.Read(buf)
				if err != nil {
					close(stdinCh)
					return
				}
			}
		}()
		<-stdinCh
	}
	return nil
}

func startSSEListener(mcpSrv *server.MCPServer, cfg *config.ServerConfig) (
	net.Listener, *http.Server, error,
) {
	addr := cfg.MCPAddr()

	serverLog.Infof("starting SSE transport on %s", addr)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		serverLog.Errorf("failed to listen on %s: %v", addr, err)
		return nil, nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	serverLog.Infof("SSE transport listening on %s", addr)

	baseURL := fmt.Sprintf("http://%s", addr)
	sseSrv := server.NewSSEServer(mcpSrv,
		server.WithBaseURL(baseURL),
	)
	httpSrv := &http.Server{Handler: sseSrv}

	go func() {
		if srvErr := httpSrv.Serve(ln); srvErr != nil &&
			srvErr != http.ErrServerClosed {
			serverLog.Errorf("SSE listener error: %v", srvErr)
		}
	}()

	return ln, httpSrv, nil
}

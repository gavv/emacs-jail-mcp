// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	logging "github.com/op/go-logging"

	"github.com/gavv/emacs-jail-mcp/internal/jail"
)

var log = logging.MustGetLogger("tools")

func Register(s *server.MCPServer, sb *jail.Jail) {
	s.AddTool(controlTool(), withLogging("control", controlHandler(sb)))
	s.AddTool(evalTool(), withLogging("eval", evalHandler(sb)))
	s.AddTool(bytecompTool(), withLogging("bytecomp", bytecompHandler(sb)))
	s.AddTool(shellTool(), withLogging("shell", shellHandler(sb)))
	s.AddTool(screenshotTool(), withLogging("screenshot", screenshotHandler(sb)))
	s.AddTool(logsTool(), withLogging("logs", logsHandler(sb)))
}

func withLogging(name string, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.Infof("request: tool=%s params=%v", name, req.Params.Arguments)

		result, err := handler(ctx, req)

		if err != nil {
			log.Errorf("reply: tool=%s error=%v", name, err)
		} else if result != nil && result.IsError {
			text := firstToolText(result)
			log.Warningf("reply: tool=%s tool_error=%s", name, truncate(text, 200))
		} else {
			text := firstToolText(result)
			log.Infof("reply: tool=%s ok (%d bytes)", name, len(text))
		}

		return result, err
	}
}

func firstToolText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return fmt.Sprintf("%s... [truncated]", s[:n])
}

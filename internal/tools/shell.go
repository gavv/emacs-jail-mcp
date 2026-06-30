// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/gavv/emacs-jail-mcp/internal/jail"
)

func shellTool() mcp.Tool {
	return mcp.NewTool("shell",
		mcp.WithDescription(
			"Run a shell command inside the jail container and return its output."),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("The shell command to execute inside the container."),
		),
	)
}

func shellHandler(sb *jail.Jail) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cmd := mcp.ParseString(req, "command", "")
		if cmd == "" {
			return mcp.NewToolResultError("command parameter is required"), nil
		}

		output, err := sb.Shell(ctx, cmd)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(output), nil
	}
}

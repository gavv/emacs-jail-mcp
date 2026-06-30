// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/gavv/emacs-jail-mcp/internal/jail"
)

func screenshotTool() mcp.Tool {
	return mcp.NewTool("screenshot",
		mcp.WithDescription(
			"Capture a screenshot of the Emacs jail display and return it as a PNG image."),
	)
}

func screenshotHandler(sb *jail.Jail) server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		b64, err := sb.ScreenshotBase64(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultImage("", b64, "image/png"), nil
	}
}

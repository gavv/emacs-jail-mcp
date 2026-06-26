// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/gavv/emacs-jail-mcp/internal/jail"
)

func evalTool() mcp.Tool {
	return mcp.NewTool("emacs_jail_eval",
		mcp.WithDescription(
			"Evaluate an Emacs Lisp expression in the jail and return the result."),
		mcp.WithString("expression",
			mcp.Required(),
			mcp.Description("The Emacs Lisp expression to evaluate."),
		),
	)
}

func evalHandler(sb *jail.Jail) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		expr := mcp.ParseString(req, "expression", "")
		if expr == "" {
			return mcp.NewToolResultError("expression parameter is required"), nil
		}

		result, err := sb.EvalElisp(ctx, expr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

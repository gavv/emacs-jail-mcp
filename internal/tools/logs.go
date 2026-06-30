// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/gavv/emacs-jail-mcp/internal/jail"
)

const autoTruncHead = 20
const autoTruncTail = 200
const defaultSources = "messages,warnings,backtrace"

// validSources lists all recognised source names.
var validSources = map[string]bool{
	"messages":          true,
	"warnings":          true,
	"backtrace":         true,
	"compile_log":       true,
	"async_compile_log": true,
	"init_log":          true,
	"stderr":            true,
}

// bufferSources maps source names that are Emacs buffers to their buffer names.
var bufferSources = map[string]string{
	"messages":          "*Messages*",
	"warnings":          "*Warnings*",
	"backtrace":         "*Backtrace*",
	"compile_log":       "*Compile-Log*",
	"async_compile_log": "*Async-native-compile-log*",
}

func logsTool() mcp.Tool {
	return mcp.NewTool("logs",
		mcp.WithDescription(
			"Return Emacs log and diagnostic output. "+
				"Default sources: messages (*Messages* buffer), "+
				"warnings (*Warnings*), backtrace (*Backtrace*). "+
				"Additional sources: init_log (Emacs init log, available before/after start), "+
				"stderr (Emacs stderr), compile_log (*Compile-Log*), "+
				"async_compile_log (*Async-native-compile-log*). "+
				"Each source section is auto-truncated (first 20 + last 200 lines). "+
				"Use offset/limit to page through a section when truncation occurs."),
		mcp.WithString("sources",
			mcp.Description(
				"Comma-separated list of sources to include. "+
					"Valid values: messages, warnings, backtrace, "+
					"compile_log, async_compile_log, init_log, stderr. "+
					"Defaults to: "+defaultSources+"."),
		),
		mcp.WithNumber("offset",
			mcp.Description("1-based line offset within each source section. "+
				"Combined with limit to read a specific window."),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of lines to return per source section. "+
				"Without offset: returns the last N lines. "+
				"With offset: returns lines starting at offset."),
		),
	)
}

func logsHandler(sb *jail.Jail) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sourcesParam := mcp.ParseString(req, "sources", "")
		if sourcesParam == "" {
			sourcesParam = defaultSources
		}

		offset := int(mcp.ParseInt64(req, "offset", 0))
		limit := int(mcp.ParseInt64(req, "limit", 0))

		// Parse and validate source list.
		rawSources := strings.Split(sourcesParam, ",")
		var sources []string
		var unknowns []string
		seen := make(map[string]bool)
		for _, s := range rawSources {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if !validSources[s] {
				unknowns = append(unknowns, s)
				continue
			}
			if !seen[s] {
				seen[s] = true
				sources = append(sources, s)
			}
		}
		if len(unknowns) > 0 {
			return mcp.NewToolResultError(fmt.Sprintf(
				"unknown source(s): %s; valid sources: "+
					"messages, warnings, backtrace, "+
					"compile_log, async_compile_log, init_log, stderr",
				strings.Join(unknowns, ", "),
			)), nil
		}
		if len(sources) == 0 {
			return mcp.NewToolResultError("no sources specified"), nil
		}

		var sections []string

		for _, src := range sources {
			lines, errMsg := fetchSource(ctx, sb, src)
			if errMsg != "" {
				sections = append(sections, fmt.Sprintf("=== %s ===\n%s", src, errMsg))
				continue
			}

			section := renderSection(src, lines, offset, limit)
			sections = append(sections, section)
		}

		return mcp.NewToolResultText(strings.Join(sections, "\n\n")), nil
	}
}

// fetchSource retrieves lines for src. Returns (nil, errorMessage) on failure.
func fetchSource(ctx context.Context, sb *jail.Jail, src string) ([]string, string) {
	switch src {
	case "init_log":
		lines := sb.LogLines()
		return lines, ""

	case "stderr":
		lines := sb.StderrLines()
		return lines, ""

	default:
		// Buffer source — requires jail to be running.
		bufName, ok := bufferSources[src]
		if !ok {
			return nil, fmt.Sprintf("(internal error: unknown buffer source %q)", src)
		}
		content, err := sb.ReadBuffer(ctx, bufName)
		if err != nil {
			return nil, fmt.Sprintf("(error reading buffer: %v)", err)
		}
		content = strings.TrimRight(content, "\n")
		if content == "" {
			return nil, ""
		}
		return strings.Split(content, "\n"), ""
	}
}

// renderSection formats a source section with header and pagination/truncation.
func renderSection(src string, lines []string, offset, limit int) string {
	header := fmt.Sprintf("=== %s ===", src)

	if len(lines) == 0 {
		return header + "\n(empty)"
	}

	total := len(lines)

	if offset > 0 || limit > 0 {
		// offset is 1-based; convert to 0-based index.
		start := 0
		if offset > 0 {
			start = offset - 1
		}
		if start >= total {
			return fmt.Sprintf("%s\n(offset %d is past end of %d lines)", header, offset, total)
		}
		window := lines[start:]
		if limit > 0 && len(window) > limit {
			window = window[:limit]
		}
		var sb strings.Builder
		sb.WriteString(header)
		sb.WriteByte('\n')
		sb.WriteString(strings.Join(window, "\n"))
		return sb.String()
	}

	// No pagination — apply auto-truncation.
	if total <= autoTruncHead+autoTruncTail {
		return header + "\n" + strings.Join(lines, "\n")
	}

	head := lines[:autoTruncHead]
	tail := lines[total-autoTruncTail:]
	omitted := total - autoTruncHead - autoTruncTail

	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteByte('\n')
	sb.WriteString(strings.Join(head, "\n"))
	fmt.Fprintf(&sb,
		"\n... [%d lines omitted; use offset=%d,limit=N or offset=1,limit=N to read more] ...\n",
		omitted, autoTruncHead+1)
	sb.WriteString(strings.Join(tail, "\n"))
	return sb.String()
}

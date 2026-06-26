// Copyright (c) Victor Gaydov and contributors
// Licensed under GPLv3+

package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/gavv/emacs-jail-mcp/internal/jail"
)

// bytecompElispTmpl is a format string for the byte-compilation elisp expression.
// %q placeholders are replaced with file_path and severity (both Go-quoted strings).
//
// The expression:
//   - byte-compiles the file without writing a persistent .elc
//   - collects errors/warnings via byte-compile-log-warning-function
//   - converts buffer positions to (line, column) pairs
//   - optionally filters by severity
//   - returns JSON with summary + diagnostics array
const bytecompElispTmpl = `
(progn
  (require 'bytecomp)
  (let* ((file (expand-file-name %q))
         (severity-filter %q)
         (buf (find-file-noselect file t))
         (elc-file nil)
         (collected nil)
         (pos-to-lc
          (lambda (b pos)
            (with-current-buffer b
              (save-excursion
                (goto-char pos)
                (cons (line-number-at-pos) (current-column))))))
         (parse-reader-pos
          (lambda (msg)
            (if (string-match ", \\([0-9]+\\), \\([0-9]+\\)$" msg)
                (cons (string-to-number (match-string 1 msg))
                      (string-to-number (match-string 2 msg)))
              (cons 1 0))))
         (byte-compile-log-warning-function
          (lambda (string position _fill level)
            (let* ((lc (if (and (numberp position)
                               (buffer-live-p buf)
                               (<= position
                                   (with-current-buffer buf (point-max))))
                           (funcall pos-to-lc buf position)
                         (funcall parse-reader-pos string)))
                   (sev (if (eq level :error) "error" "warning")))
              (push (list (cons 'severity sev)
                          (cons 'line (car lc))
                          (cons 'column (cdr lc))
                          (cons 'message string))
                    collected))))
         (byte-compile-dest-file-function
          (lambda (_f) (make-temp-file "emacs-jail-bytecomp-" nil ".elc"))))
    (unwind-protect
        (progn
          (with-current-buffer buf
            (revert-buffer t t t))
          (setq elc-file (byte-compile-dest-file file))
          (byte-compile-from-buffer buf))
      (when (and elc-file (file-exists-p elc-file))
        (delete-file elc-file)))
    (let* ((diags (nreverse collected))
           (filtered (if (and (stringp severity-filter)
                              (not (string= severity-filter "")))
                         (seq-filter
                          (lambda (d) (string= (cdr (assq 'severity d))
                                               severity-filter))
                          diags)
                       diags))
           (nerrors (length (seq-filter
                             (lambda (d) (string= (cdr (assq 'severity d)) "error"))
                             filtered)))
           (nwarnings (length (seq-filter
                               (lambda (d) (string= (cdr (assq 'severity d)) "warning"))
                               filtered))))
      (json-encode
       (list (cons 'file file)
             (cons 'summary (list (cons 'total (length filtered))
                                  (cons 'errors nerrors)
                                  (cons 'warnings nwarnings)))
             (cons 'diagnostics (vconcat filtered)))))))
`

func bytecompTool() mcp.Tool {
	return mcp.NewTool("emacs_jail_bytecomp",
		mcp.WithDescription(
			"Byte-compile an elisp file synchronously and return errors and warnings. "+
				"No .elc file is written. Results are sorted by position."),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path to the elisp file to check."),
		),
		mcp.WithString("severity",
			mcp.Description("Optional filter by severity level."),
			mcp.Enum("error", "warning"),
		),
	)
}

func bytecompHandler(sb *jail.Jail) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filePath := mcp.ParseString(req, "file_path", "")
		if filePath == "" {
			return mcp.NewToolResultError("file_path parameter is required"), nil
		}

		severity := mcp.ParseString(req, "severity", "")

		expr := fmt.Sprintf(bytecompElispTmpl, filePath, severity)

		result, err := sb.EvalElisp(ctx, expr)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

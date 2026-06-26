;;; site-start.el --- Stream init messages to log file and stderr -*- lexical-binding: t; -*-

;; Copyright (C) Victor Gaydov and contributors
;; Licensed under GPLv3+

;; Loaded by Emacs before user init (via EMACSLOADPATH prepend).
;; Instruments message, display-warning, and load so that elisp messages,
;; warnings, and file loading events (including failures with backtraces)
;; are streamed to emacs-jail-log-path and stderr immediately during startup.

;;; Code:

(defvar emacs-jail t
  "Non-nil when Emacs is running inside the emacs-jail-mcp container.
Set early in site-start.el so user init can detect the jail environment.")

;; Initialize logging advices.
(require 'emacs-jail-log)

(provide 'site-start)

;;; site-start.el ends here

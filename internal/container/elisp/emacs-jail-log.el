;;; emacs-jail-log.el --- Stream init messages to log file and stderr -*- lexical-binding: t; -*-

;; Copyright (C) Victor Gaydov and contributors
;; Licensed under GPLv3+

;; Instruments message, display-warning, and load so that elisp messages,
;; warnings, and file loading events (including failures with backtraces)
;; are streamed to emacs-jail-log-path and stderr immediately during startup.

;;; Code:

(defvar emacs-jail-log-path
  (or (getenv "EMACS_JAIL_LOG_PATH") nil)
  "Path to the log file where init messages are streamed.
Initialised from the EMACS_JAIL_LOG_PATH environment variable,
which is set by the emacs-jail-mcp entrypoint before Emacs starts.")

(defun emacs-jail-log--write-file (line)
  "Append LINE followed by a newline to `emacs-jail-log-path'."
  (when (and emacs-jail-log-path (stringp emacs-jail-log-path))
    (condition-case nil
        (write-region (concat line "\n") nil emacs-jail-log-path t 'silent)
      (error nil))))

(defun emacs-jail-log--write-stderr (line)
  "Write LINE followed by a newline to stderr."
  (condition-case nil
      (mapc #'external-debugging-output (concat line "\n"))
    (error nil)))

;;; Stream every (message ...) call

(defun emacs-jail-log--message-advice (orig-fn format-string &rest args)
  "Around advice for `message': call ORIG-FN then log the result."
  (let ((result (apply orig-fn format-string args)))
    (when (and format-string (not (string= format-string "")))
      (let ((text (condition-case nil
                      (apply #'format format-string args)
                    (error format-string))))
        (emacs-jail-log--write-file text)
        (emacs-jail-log--write-stderr (concat "(*Messages*) " text))))
    result))

(advice-add 'message :around #'emacs-jail-log--message-advice)

;;; Stream every display-warning call

(defun emacs-jail-log--warning-advice (orig-fn type message &optional level buffer-name)
  "Around advice for `display-warning': call ORIG-FN then write to stderr."
  (let ((result (funcall orig-fn type message level buffer-name)))
    (emacs-jail-log--write-stderr
     (format "(*Warnings*) (%s) %s: %s"
             (symbol-name (or level :warning))
             type
             message))
    result))

(advice-add 'display-warning :around #'emacs-jail-log--warning-advice)

;;; Log every load with entry, exit, or failure (with backtrace on failure)

(defun emacs-jail-log--load-advice (orig-fn file &rest args)
  "Around advice for `load': log entry and exit to the log file.
On failure, log the error message and a backtrace, then re-signal."
  (emacs-jail-log--write-file (format "load> %s" file))
  (condition-case err
      (let ((result (apply orig-fn file args)))
        (emacs-jail-log--write-file (format "load< %s" file))
        result)
    (error
     (emacs-jail-log--write-file
      (format "load! %s: %s" file (error-message-string err)))
     (emacs-jail-log--write-stderr
      (format "[load-error] %s: %s" file (error-message-string err)))
     (condition-case nil
         (let ((bt (with-output-to-string (backtrace))))
           (dolist (line (split-string bt "\n" t))
             (emacs-jail-log--write-file (concat "  " line))))
       (error nil))
     (signal (car err) (cdr err)))))

(advice-add 'load :around #'emacs-jail-log--load-advice)

(provide 'emacs-jail-log)

;;; emacs-jail-log ends here

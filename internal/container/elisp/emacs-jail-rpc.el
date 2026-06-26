;;; emacs-jail-rpc.el --- Emacs Jail RPC Server -*- lexical-binding: t; -*-

;; Copyright (C) Victor Gaydov and contributors
;; Licensed under GPLv3+
;;
;; Based on code from emacs-mcp-server by Rolf Håvard Blindheim
;; <https://github.com/rhblind/emacs-mcp-server/>
;; Licensed under GPLv3

;; This is the RPC server loaded by emacs-jail-mcp inside the container.
;; It provides a single JSON-RPC method, `eval`, that evaluates arbitrary Elisp
;; with all security disabled (the jail runs inside a throw-away Podman container).
;;
;; Wire protocol: newline-delimited JSON-RPC 2.0 over a Unix domain socket.
;; The only supported method is "eval":
;;   Request:  {"jsonrpc":"2.0","id":<n>,"method":"eval","params":{"expression":"<elisp>"}}
;;   Response: {"jsonrpc":"2.0","id":<n>,"result":{"value":"<string>"}}
;;   Error:    {"jsonrpc":"2.0","id":<n>,"result":{"error":"<message>"}}

;;; Code:

(require 'json)

;;; Variables

(defvar emacs-jail-rpc--running nil
  "Whether the emacs jail rpc server is currently running.")

(defvar emacs-jail-rpc-debug nil
  "Whether to enable debug logging.")

;; Transport state

(defvar emacs-jail-rpc--server-process nil
  "The Unix domain socket server process.")

(defvar emacs-jail-rpc--client-process nil
  "The single connected client process, or nil.")

(defvar emacs-jail-rpc--client-buffer ""
  "Line buffer accumulating partial data from the client.")

;;; Line Buffer

(defun emacs-jail-rpc--process-buffer-lines (buffer new-data line-processor)
  "Process lines in BUFFER with NEW-DATA using LINE-PROCESSOR.
Returns updated buffer with remaining partial data."
  (let ((combined (concat buffer new-data)))
    (while (string-match "\n" combined)
      (let* ((line-end (match-end 0))
             (line (substring combined 0 (1- line-end))))
        (setq combined (substring combined line-end))
        (when (> (length (string-trim line)) 0)
          (condition-case err
              (funcall line-processor line)
            (error
             (when emacs-jail-rpc-debug
               (message "[EMACS-JAIL] Error processing line: %s"
                        (error-message-string err))))))))
    combined))

;;; Unix Socket Server — Process Handlers

(defun emacs-jail-rpc--server-sentinel (process event)
  "Handle server PROCESS sentinel EVENT."
  (cond
   ((string-match "open.*" event)
    (emacs-jail-rpc--handle-new-connection process))
   ((memq (process-status process) '(exit signal))
    (when emacs-jail-rpc-debug
      (message "[EMACS-JAIL] Unix socket server process terminated: %s" event))
    (emacs-jail-rpc--stop-server)
    (setq emacs-jail-rpc--running nil))
   (t
    (when emacs-jail-rpc-debug
      (message "[EMACS-JAIL] Unix socket server process event: %s" event)))))

(defun emacs-jail-rpc--client-filter (process string)
  "Handle incoming data from client PROCESS with STRING."
  (when (eq process emacs-jail-rpc--client-process)
    (setq emacs-jail-rpc--client-buffer
          (emacs-jail-rpc--process-buffer-lines
           emacs-jail-rpc--client-buffer
           string
           #'emacs-jail-rpc--process-message))))

(defun emacs-jail-rpc--client-sentinel (process event)
  "Handle client PROCESS sentinel EVENT."
  (when (eq process emacs-jail-rpc--client-process)
    (when emacs-jail-rpc-debug
      (message "[EMACS-JAIL] Client disconnected: %s" (string-trim event)))
    (setq emacs-jail-rpc--client-process nil)))

(defun emacs-jail-rpc--handle-new-connection (client-process)
  "Handle new connection from CLIENT-PROCESS."
  ;; Defensive: disconnect any stale client.
  (when (and emacs-jail-rpc--client-process
             (processp emacs-jail-rpc--client-process))
    (when emacs-jail-rpc-debug
      (message "[EMACS-JAIL] Replacing existing client connection"))
    (delete-process emacs-jail-rpc--client-process))
  (when emacs-jail-rpc-debug
    (message "[EMACS-JAIL] New Unix socket client connected"))
  (set-process-filter client-process #'emacs-jail-rpc--client-filter)
  (set-process-sentinel client-process #'emacs-jail-rpc--client-sentinel)
  (set-process-coding-system client-process 'utf-8 'utf-8)
  (setq emacs-jail-rpc--client-process client-process)
  (setq emacs-jail-rpc--client-buffer ""))

;;; Unix Socket Server — Message Processing

(defun emacs-jail-rpc--process-message (line)
  "Process a message LINE from the client."
  (condition-case err
      (let ((message (json-parse-string line :object-type 'alist :array-type 'list)))
        (unless (string= (alist-get 'jsonrpc message) "2.0")
          (error "Invalid or missing jsonrpc version"))
        (emacs-jail-rpc--handle-message message))
    (error
     (when emacs-jail-rpc-debug
       (message "[EMACS-JAIL] Error processing message: %s" (error-message-string err)))
     (emacs-jail-rpc--send-error-response
      nil -32700 "Parse error" (error-message-string err)))))

;;; Unix Socket Server — Send Helpers

(defun emacs-jail-rpc--send-error-response (id code message &optional _data)
  "Send JSON-RPC error response with ID, CODE, and MESSAGE."
  (when (and emacs-jail-rpc--client-process
             (eq (process-status emacs-jail-rpc--client-process) 'open))
    (process-send-string
     emacs-jail-rpc--client-process
     (format "{\"jsonrpc\":\"2.0\",\"id\":%s,\"error\":{\"code\":%d,\"message\":\"%s\"}}\n"
             (if id (number-to-string id) "null")
             code
             message))))

(defun emacs-jail-rpc--send-response (json-string)
  "Send raw JSON-STRING to the client."
  (when (and emacs-jail-rpc--client-process
             (eq (process-status emacs-jail-rpc--client-process) 'open))
    (condition-case err
        (process-send-string emacs-jail-rpc--client-process (concat json-string "\n"))
      (error
       (when emacs-jail-rpc-debug
         (message "[EMACS-JAIL] Error sending response: %s" (error-message-string err)))))))

;;; Unix Socket Server — Lifecycle

(defun emacs-jail-rpc--start-server (socket-path)
  "Start Unix domain socket server at SOCKET-PATH."
  ;; Clean up any existing socket file.
  (when (and socket-path (file-exists-p socket-path))
    (delete-file socket-path))

  (setq emacs-jail-rpc--client-process nil)
  (setq emacs-jail-rpc--client-buffer "")

  (condition-case err
      (progn
        (setq emacs-jail-rpc--server-process
              (make-network-process
               :name "emacs-jail-rpc-server"
               :family 'local
               :service socket-path
               :server t
               :filter #'ignore
               :sentinel #'emacs-jail-rpc--server-sentinel
               :coding 'utf-8))
        (when (file-exists-p socket-path)
          (set-file-modes socket-path #o600))
        (when emacs-jail-rpc-debug
          (message "[EMACS-JAIL] Unix socket server started at: %s" socket-path)))
    (error
     (when (and socket-path (file-exists-p socket-path))
       (delete-file socket-path))
     (error "Failed to start Unix socket server: %s" (error-message-string err)))))

(defun emacs-jail-rpc--stop-server ()
  "Stop the Unix domain socket server."
  (when (and emacs-jail-rpc--client-process (processp emacs-jail-rpc--client-process))
    (delete-process emacs-jail-rpc--client-process)
    (setq emacs-jail-rpc--client-process nil))
  (setq emacs-jail-rpc--client-buffer "")
  (when (processp emacs-jail-rpc--server-process)
    (let ((socket-path (process-contact emacs-jail-rpc--server-process :local)))
      (delete-process emacs-jail-rpc--server-process)
      (setq emacs-jail-rpc--server-process nil)
      (when (and socket-path (file-exists-p socket-path))
        (condition-case nil
            (delete-file socket-path)
          (error nil))))))

;;; Eval Handler

(defun emacs-jail-rpc--eval-expression (expression)
  "Evaluate EXPRESSION string and return the result as a string.
Strings are returned as-is; all other values use `%S' format."
  (let* ((form (car (read-from-string expression)))
         (result (eval form t)))
    ;; Use %s for strings so the caller receives the raw string value
    ;; without an extra layer of Lisp quoting.  Use %S for everything
    ;; else so structure is preserved (lists, symbols, numbers, etc.).
    (if (stringp result)
        result
      (format "%S" result))))

(defun emacs-jail-rpc--handle-message (message)
  "Handle incoming JSON-RPC MESSAGE.
Dispatches the `eval' method; returns -32601 for unknown methods."
  (when emacs-jail-rpc-debug
    (message "[EMACS-JAIL] Handling message: %s" message))
  (condition-case err
      (let ((method (alist-get 'method message))
            (id (alist-get 'id message))
            (params (alist-get 'params message)))
        (cond
         ((string= method "eval")
          (emacs-jail-rpc--handle-eval id params))
         (t
          (emacs-jail-rpc--send-error-response
           id -32601
           (format "Method not found: %s" (or method "<nil>"))))))
    (error
     (when emacs-jail-rpc-debug
       (message "[EMACS-JAIL] Error handling message: %s" err))
     (condition-case _send-err
         (emacs-jail-rpc--send-error-response
          (alist-get 'id message) -32603
          "Internal error")
       (error nil)))))

(defun emacs-jail-rpc--handle-eval (id params)
  "Handle eval request with ID and PARAMS."
  (let ((expression (alist-get 'expression params)))
    (if (not expression)
        (emacs-jail-rpc--send-error-response id -32602
                                          "Missing 'expression' parameter")
      (condition-case err
          (let* ((value (emacs-jail-rpc--eval-expression expression))
                 (result-hash (make-hash-table :test 'equal))
                 (response-hash (make-hash-table :test 'equal)))
            (puthash "value" value result-hash)
            (puthash "jsonrpc" "2.0" response-hash)
            (puthash "id" id response-hash)
            (puthash "result" result-hash response-hash)
            (emacs-jail-rpc--send-response (json-serialize response-hash)))
        (error
         (let* ((result-hash (make-hash-table :test 'equal))
                (response-hash (make-hash-table :test 'equal)))
           (puthash "error" (format "Error: %s" (error-message-string err)) result-hash)
           (puthash "jsonrpc" "2.0" response-hash)
           (puthash "id" id response-hash)
           (puthash "result" result-hash response-hash)
           (emacs-jail-rpc--send-response (json-serialize response-hash))))))))

;;; Public API

(defun emacs-jail-rpc-start (socket-path)
  "Start emacs jail rpc server listening on SOCKET-PATH."
  (when emacs-jail-rpc--running
    (error "Emacs jail rpc server is already running"))
  (unless socket-path
    (error "socket-path argument is required"))

  (condition-case err
      (progn
        (emacs-jail-rpc--start-server socket-path)
        (setq emacs-jail-rpc--running t))
    (error
     (error "Failed to start emacs jail rpc server: %s" (error-message-string err)))))

(defun emacs-jail-rpc-stop ()
  "Stop the emacs jail rpc server."
  (unless emacs-jail-rpc--running
    (error "Emacs jail rpc server is not running"))
  (emacs-jail-rpc--stop-server)
  (setq emacs-jail-rpc--running nil))

(provide 'emacs-jail-rpc)

;;; emacs-jail-rpc.el ends here

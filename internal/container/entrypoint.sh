#!/bin/bash
set -e

# Watchdog: self-terminate when the parent process exits
_EMACS_JAIL_PGID=$(ps -o pgid= -p $$ | tr -d ' ')
(
  while ! flock --nonblock "$EMACS_JAIL_LOCK_PATH" true 2>/dev/null; do sleep 1; done
  rm -f "$EMACS_JAIL_SOCKET_PATH" "$EMACS_JAIL_LOG_PATH" "$EMACS_JAIL_STDERR_PATH"
  kill -TERM -"$_EMACS_JAIL_PGID" 2>/dev/null || true
) &

# Start Xvfb
Xvfb "$EMACS_JAIL_DISPLAY" -screen 0 "${EMACS_JAIL_DISPLAY_WIDTH}x${EMACS_JAIL_DISPLAY_HEIGHT}x${EMACS_JAIL_DISPLAY_DEPTH}" -nolisten tcp -ac &
sleep 0.5

# Wait for X11 socket
for i in $(seq 1 20); do
  [ -e "/tmp/.X11-unix/X${EMACS_JAIL_DISPLAY_NUMBER}" ] && break
  sleep 0.25
done

export DISPLAY="$EMACS_JAIL_DISPLAY"

# Remove stale socket and log
rm -f "$EMACS_JAIL_SOCKET_PATH" "$EMACS_JAIL_LOG_PATH" "$EMACS_JAIL_STDERR_PATH"

# Stream elisp messages to log via site-start.el
export EMACSLOADPATH="${EMACS_JAIL_ELISP_DIR}:"

# Start Emacs with emacs-jail-rpc server
export EMACS_JAIL_LOG_PATH="$EMACS_JAIL_LOG_PATH"
exec $EMACS_JAIL_EMACS_BINARY --maximized --eval "(progn
  (add-to-list (quote load-path) \"${EMACS_JAIL_ELISP_DIR}\")
  (require (quote emacs-jail-rpc))
  (emacs-jail-rpc-start \"${EMACS_JAIL_SOCKET_PATH}\"))" 2>"$EMACS_JAIL_STDERR_PATH"

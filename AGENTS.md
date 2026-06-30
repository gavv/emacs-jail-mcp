# AI agent instructions

## Coding guidelines

Code style:

- Do not use banner comments like `// ---- Foo ----` to separate sections. Use blank lines and normal prose comments or no comment at all.
- Avoid lines longer than 90 characters.

Tests:

- Do not write tests that mirror the implementation 1-to-1 (e.g. asserting that fmt.Sprintf produces the string it produces, or that struct literals equal themselves). Tests must verify observable behavior, error paths, edge cases, or integration — not repeat the source code in test form.
- Do not use t.Parallel() in tests.
- Every MCP tool must have a dedicated e2e subtest inside TestMCP in e2e/e2e_test.go. The subtest must run after Start and before Stop, exercise the tool against the live jail, and verify observable behavior (not just that the call succeeds).
- Every client CLI command must have a dedicated e2e subtest inside TestCLI in e2e/e2e_test.go. The subtest must exercise the command against the live jail and verify observable behavior.

Documentation:

- When architecture, behavior, or important implementation details change, update AGENTS.md to reflect them.
- When a tricky bug or non-obvious behavior is discovered, document it in the "Pitfalls & lessons learned" section of AGENTS.md.

## About project

A Go-based MCP (Model Context Protocol) server that runs Emacs inside a disposable Podman container. The server proxies elisp evaluation to an embedded `emacs-jail-rpc` elisp server running inside the container over a Unix domain socket.

Transport is selected via the `--stdio` flag on the `serve` subcommand: with `--stdio` it uses stdio (for LLM clients that launch the binary as a subprocess); without it (the default), it listens on a TCP port using SSE transport (for the `send` CLI subcommand and programmatic clients).

The user's real Emacs config is loaded inside the container, but the MCP server configuration is overridden so that the jail runs its own `emacs-jail-rpc` server instead of whatever the user has configured (e.g. `emacs-mcp-server`).

One Go process = one Podman container = one Emacs instance.

## Architecture overview

```
LLM Client (stdio mode)     LLM Client (sse mode) / CLI client / E2E tests
    |                                      |
    | stdin/stdout (MCP, --stdio flag)     | TCP (MCP SSE transport)
    v                                      v
emacs-jail-mcp (Go binary, serve subcommand)
    |
    | Unix domain socket (newline-delimited JSON-RPC 2.0)
    v
emacs-jail-rpc (Elisp server inside Emacs inside Podman container)
    |
    | Xvfb virtual display
    v
Emacs GUI (headless, screenshottable)
```

### Key components

- **Go binary** (`cmd/emacs-jail-mcp/main.go`): Entry point. Initialises logging and delegates to `internal/cli`. Uses cobra for subcommand dispatch. Subcommands: `serve` (start MCP server), `send` (invoke individual tools against a running server), `info` (show MCP server address and whether it is responding), and `man` (generate manual pages). Handles SIGINT/SIGTERM for clean shutdown.

- **CLI** (`internal/cli/`): Cobra subcommand implementations. `serve` starts the MCP server — with `--stdio` it uses stdio transport, without it (default) it listens on a TCP port using SSE transport, staying alive until stdin closes. `send` proxies individual tool calls to a running server over SSE/TCP. `info` dials the configured TCP address and reports whether the server is responding. `man` emits troff or Markdown documentation using `cobradoc`.

- **Config** (`internal/config/`): All public settings via CLI flags, no config file. `server.go` defines `ServerConfig` with derived helpers: `JailID`, `ContainerName`, `SocketPath`, `MCPAddr`, `ElispDir`, `LogPath`, `StderrPath`, and `LockPath`. `display.go` defines display defaults and CLI overrides. `client.go` defines `ClientConfig` (host/port for the CLI client). `ports.go` holds port constants.

- **Display** (`internal/display/`): Xvfb display configuration. Chooses the display number, queries the host display size with `xdpyinfo` when available, falls back to default dimensions, applies explicit CLI overrides, and provides X11 socket/lock paths. Host display probing is an internal jail concern and is not performed by the CLI.

- **Container** (`internal/container/`): Podman lifecycle. `Start()` writes embedded elisp files to `/tmp`, acquires the watchdog lock, runs `podman run` with the generated entrypoint script and jail-selected display config, and forwards `SHELL` when set. `Stop()` kills and forcibly removes the container, releases the lock, removes embedded elisp files, and clears stale Xvfb socket/lock files. `Exec()`/`ExecRaw()` run commands inside. `LogLines()` and `StderrLines()` read shared `/tmp` files directly from the host.

- **Entrypoint** (`internal/container/entrypoint.go`, `entrypoint.sh`): A static bash script (`entrypoint.sh`) embedded via `go:embed`. `entrypoint.go` generates the environment variables passed to the script (socket path, log/stderr paths, display settings, elisp dir, etc.). The script: (1) starts watchdog, (2) starts Xvfb, (3) waits for X11 socket, (4) sets `DISPLAY`, `EMACSLOADPATH`, and `EMACS_JAIL_LOG_PATH`, (5) removes stale socket/log/stderr files, (6) exec's Emacs with `--maximized --eval` to load and start `emacs-jail-rpc`. Emacs stderr is captured to a shared `/tmp` file.

- **Emacs elisp** (`internal/container/elisp/`): Embedded via `go:embed` and written before container start. `emacs-jail-rpc.el` is the eval server over a Unix domain socket. `site-start.el` sets `emacs-jail` early and loads `emacs-jail-log.el`. `emacs-jail-log.el` instruments init diagnostics.

- **Init instrumentation** (`site-start.el`, `emacs-jail-log.el`): Loaded before user init via `EMACSLOADPATH` prepend. Sets `emacs-jail` to `t`; advises `message`, `display-warning`, and `load`; writes init diagnostics to `EMACS_JAIL_LOG_PATH`; mirrors messages, warnings, and load failures to Emacs stderr.

- **Client** (`internal/emacsclient/`): Go-side Unix socket client. `Client` implements newline-delimited JSON-RPC 2.0 over a Unix domain socket. `EvalElisp()` serializes the full request/response cycle with a mutex because MCP handlers can call it concurrently while the underlying connection and `bufio.Reader` are shared.

- **Jail** (`internal/jail/`): Orchestrator with state machine (`Stopped -> Starting -> Running -> Stopping -> Stopped`). Ties together container, display, and MCP client. Public methods are concurrency-safe. Accumulates init log and stderr lines during `waitForSocket()`, and streams Emacs stderr to the application logger while running.

- **Tools** (`internal/tools/`): 6 MCP tools registered on the Go-side MCP server:
  - `emacs_jail_control` — controls jail lifecycle: start (launch container, wait for socket, connect client), stop (disconnect client, kill container), restart (stop + clear logs + start), status (return current state)
  - `emacs_jail_logs` — returns log/diagnostic output from multiple sources (init log, stderr, Emacs buffers); supports source selection, pagination, and auto-truncation
  - `emacs_jail_eval` — evaluates elisp via emacs-jail-rpc
  - `emacs_jail_bytecomp` — byte-compiles an elisp file (without writing a `.elc`), returns errors and warnings as JSON with file, line, column, message, and severity
  - `emacs_jail_shell` — runs shell command inside container (with DISPLAY)
  - `emacs_jail_screenshot` — captures Xvfb display as PNG via ImageMagick `import`

## Container setup

Uses `podman run --rootfs /:O` — the host root filesystem with a CoW (copy-on-write) overlay. No Dockerfile, no image build. Combined with `--uts=host`, `--network=host`, `--ipc=host`, and `--security-opt label=disable`, the container has near-full host access while remaining disposable. The container runs in its own private PID namespace (no `--pid=host`). `--detach` runs the container in the background; `--tty` allocates a PTY (required for shell commands and `tty` detection); `--init` runs an init process to reap zombies.

Volumes:
- `/tmp:/tmp` — shared mount for socket, log file, and elisp files
- `/run/user/<uid>:/run/user/<uid>` — for D-Bus and other user runtime files
- `$HOME/.cache:$HOME/.cache` — Emacs package cache

The container runs as the current user (`--user <uid>:<gid>`) and in the current working directory (`--workdir <cwd>`). Container name is `emacs-jail-<pid>` and auto-removes on exit (`--rm`).

## Instrumentation

The files in `internal/container/elisp/` are written to a temporary directory and that directory is prepended to `EMACSLOADPATH` (with trailing colon to keep default dirs). Emacs loads `site-start.el` **before** user init, making it the right place to install instrumentation.

The instrumentation does four things:

1. **Sets `emacs-jail` to `t`** — user init can detect the jail with `(bound-and-true-p emacs-jail)` and skip problematic code.

2. **Advises `message`** (`:around`) — every `(message ...)` call is logged to the init log file and mirrored to stderr with a `(*Messages*)` prefix.

3. **Advises `display-warning`** (`:around`) — warnings are mirrored to stderr with a `(*Warnings*)` prefix.

4. **Advises `load`** (`:around`) — every file load is logged with entry (`load>`), exit (`load<`), or failure (`load!` with error + backtrace). Only fires on actual `load` failures, not caught exceptions.

The log file path comes from `EMACS_JAIL_LOG_PATH`, set in the entrypoint before Emacs starts. The init log and stderr files live in `/tmp` (shared mount) and are readable from the host without `podman exec`.

**Why not `--eval` or `after-init-hook`?** They run too late — after user init has already loaded. We need to capture messages, warnings, and load events *during* init.

**Why not advise `signal`?** It fires on every `condition-case`-caught error, creating massive noise. We only care about uncaught load failures.

## Watchdog

The container runs in a private PID namespace, so `/proc/<hostPID>` is not visible inside the container. Instead, the Go process acquires an exclusive `flock(LOCK_EX)` on a sentinel file (`/tmp/emacs-jail-<pid>.lock`) before starting the container. The entrypoint spawns a background loop:

```bash
_EMACS_JAIL_PGID=$(ps -o pgid= -p $$ | tr -d ' ')
(
  while ! flock --nonblock '<lockfile>' true 2>/dev/null; do sleep 1; done
  rm -f '<socket>' '<log>' '<stderr>'
  kill -TERM -"$_EMACS_JAIL_PGID" 2>/dev/null || true
) &
```

When the parent Go process disappears (e.g. SIGKILL), the OS automatically releases the flock. The watchdog's next `flock --nonblock` call succeeds, signalling that the parent is gone — at which point it kills the container's entire process group and cleans up socket/log/stderr files. Combined with `--rm`, this ensures no orphan containers are left.

The lock file lives in `/tmp` (shared volume mount), so it is visible to the container without requiring a shared PID namespace.

## Wire protocol

Go <-> Emacs communication uses newline-delimited JSON-RPC 2.0 over a Unix domain socket at `/tmp/emacs-jail-<pid>.sock`.

The Go side (`Client`) sends sequential JSON-RPC requests over a Unix domain socket:
- `Connect()` dials the socket
- `EvalElisp()` writes request JSON + newline, reads one response line, unmarshals it
- Context cancellation closes the connection to unblock the pending read

The Emacs side (`emacs-jail-rpc`) uses `make-network-process :server t :family 'local` to listen. Line-buffered protocol: accumulate data until newline, parse JSON, eval synchronously, respond. One request is processed at a time.

## Eval semantics

The `eval-elisp` tool handler in Emacs (`emacs-jail-rpc.el`) returns:
- **Strings as-is** — `(concat "hel" "lo")` returns `hello`, not `"hello"`
- **Everything else via `%S`** — preserves structure for lists, symbols, numbers, etc.

This avoids double-quoting issues where the LLM would see `"\"hello\""` instead of `"hello"`.

## Security

All security is intentionally disabled — the jail runs in a throw-away Podman container. The `eval-elisp` handler calls `(eval form t)` directly with no filtering.

## Dependencies

- **Go**: `mcp-go`, `cobra`, `cobradoc`, `op/go-logging`, `testify`
- **System**: Podman (with sudo), Xvfb, ImageMagick (`import` command), Emacs
- **Elisp**: Originally adapted from `rhblind/emacs-mcp-server`

## Build & test

```
task          # tidy + build + lint + docs + unit tests + e2e tests
task build    # go build -> bin/emacs-jail-mcp
task docs     # regenerate MANUAL.md and doc/emacs-jail-mcp.1
task test     # go test ./internal/...
task e2e      # go test -count=1 -tags e2e -v -timeout 120s ./e2e/
task lint     # golangci-lint run ./...
```

E2E tests start an in-process server by running `cli.NewRootCmd()` with args `["serve", "--mcp-port", ...]` in a goroutine (see `e2e/server_test.go`). Tests connect using `mcpclient.NewSSEMCPClient` over TCP/SSE.

`TestMCP` in `e2e/mcp_test.go` runs the full tool lifecycle:
ListTools -> EvalBeforeStart -> Control/StatusBeforeStart -> Control/Start -> Logs/InitLog -> Logs/Stderr -> Logs/Buffers -> Logs/AllSources -> Eval/Simple -> Eval/String -> Eval/Version -> Eval/ShellCommand -> Eval/Tty -> Bytecomp/NoFilter -> Bytecomp/FilterWarning -> Bytecomp/FilterError -> Bytecomp/NonexistentFile -> Shell/Echo -> Shell/Display -> Shell/Tty -> Screenshot -> Control/StatusWhileRunning -> Control/Restart -> Eval/AfterRestart -> Control/Stop -> EvalAfterStop -> Control/StatusAfterStop.

`TestCLI` in `e2e/cli_test.go` exercises CLI subcommands:
InfoBeforeStart -> InfoAfterStart -> Control/Start -> Eval -> Shell -> Logs -> Bytecomp -> Screenshot -> Control/Stop.

Init logs are captured after Start and printed via `t.Log` only on test failure.

## File layout

```
cmd/emacs-jail-mcp/main.go               # Entry point (logging init, cli.NewRootCmd)

internal/cli/root.go                        # Root cobra command
internal/cli/serve.go                       # serve subcommand (--stdio / SSE transport)
internal/cli/send.go                        # send subcommand + tool sub-subcommands
internal/cli/info.go                        # info subcommand
internal/cli/man.go                         # man subcommand (troff / Markdown docs)

internal/config/server.go                   # ServerConfig, CLI flags, derived paths
internal/config/server_test.go
internal/config/display.go                  # DisplayConfig and defaults
internal/config/client.go                   # ClientConfig (host/port for CLI client)
internal/config/client_test.go
internal/config/ports.go                    # Port constants

internal/display/display.go                 # Xvfb display config and host size detection
internal/display/display_test.go

internal/container/container.go             # Podman lifecycle
internal/container/container_test.go
internal/container/entrypoint.go            # Environment variables for entrypoint script
internal/container/entrypoint.sh            # Static embedded bash entrypoint script
internal/container/entrypoint_test.go
internal/container/elisp.go                 # go:embed + WriteElispFiles

internal/container/elisp/site-start.el      # Pre-init instrumentation
internal/container/elisp/emacs-jail-log.el  # Init log/stderr instrumentation
internal/container/elisp/emacs-jail-rpc.el  # Eval server (JSON-RPC over Unix socket)

internal/jail/jail.go                       # Orchestrator, state machine
internal/jail/jail_test.go
internal/jail/stderr_streamer.go            # Stderr log streaming
internal/jail/stderr_streamer_test.go

internal/emacsclient/client.go              # Client (JSON-RPC over Unix socket)
internal/emacsclient/client_test.go

internal/tools/tools.go                     # Register all 6 tools
internal/tools/control.go
internal/tools/logs.go
internal/tools/eval.go
internal/tools/bytecomp.go
internal/tools/shell.go
internal/tools/screenshot.go
internal/tools/tools_test.go

e2e/main_test.go                            # TestMain with log capture
e2e/server_test.go                          # TestServer helper (start/stop in-process server)
e2e/mcp_test.go                             # TestMCP: full MCP tool lifecycle via SSE client
e2e/cli_test.go                             # TestCLI: CLI subcommands against live jail
```

## Pitfalls & lessons learned

1. **`EMACSLOADPATH` trailing colon**: `EMACSLOADPATH='<dir>:'` prepends `<dir>` while keeping all default directories. Without the trailing colon, Emacs loses its default load-path and can't find built-in packages.

2. **Log path must be an environment variable**: `--eval` and `after-init-hook` run after user init, so they are too late to configure init logging. `EMACS_JAIL_LOG_PATH` must be set before Emacs starts so `site-start.el` can read it.

3. **`signal` advice is too noisy**: Advising `signal` fires on every `condition-case`-caught error. Log actual `load` failures instead, where the error is uncaught by the loaded file.

4. **Read logs from host files**: Init log and stderr files live in `/tmp` (shared mount), so `Container.LogLines()` and `Container.StderrLines()` read them directly from the host. This avoids `podman exec` and works before the jail is fully started.

5. **`podman exec` doesn't inherit entrypoint env**: Environment variables set by the container entrypoint, such as `DISPLAY`, are not available in `podman exec`. Pass them explicitly with `-e`, e.g. `-e DISPLAY=:NN`.

6. **Forward `PATH` and `SHELL` into the container**: `PATH` ensures tools installed by the parent process (for example a matrix-selected Emacs in CI) are visible when the entrypoint launches `emacs`. Without `SHELL`, Emacs can fall back to `/bin/sh`. If user config sets `shell-command-switch` to `"-ic"`, dash may print `can't access tty; job control turned off`; forwarding host `SHELL` lets Emacs use the user's shell.

7. **Watchdog uses flock, not `/proc`**: The container has a private PID namespace, so it can't see `/proc/<hostPID>`. The Go process holds an exclusive lock on a sentinel file in `/tmp`; when the parent exits, the OS releases the lock and the watchdog kills the container process group.

8. **Restart needs force cleanup**: `Stop()` uses `podman rm --force --ignore` after `podman kill` so the reused container name is freed immediately. It also removes stale Xvfb socket/lock files so the same display number can be reused.

9. **`emacs-version` format varies**: Some builds return `"30.1"` instead of `"GNU Emacs 30.1..."`. Tests should check for a version-like string, not a specific prefix.

10. **Eval string quoting**: Returning strings via `(format "%S" result)` wraps them in quotes (`"\"hello\""`). Return strings as-is and use `%S` only for non-strings.

11. **Jail needs its own RWMutex**: MCP handlers run concurrently and share one `Jail`. Use an exclusive lock for state-changing methods (`Start`, `Stop`, `Restart`) and a read lock for readers (`EvalElisp`, `Shell`, `screenshot`, logs). Keep `Restart` on internal locked helpers to avoid recursive locking.

12. **Concurrent `EvalElisp` corrupts `bufio.Reader`**: MCP handlers can call `EvalElisp` from different goroutines. `Client.EvalElisp` must hold a mutex for the whole write/read cycle because the connection and `bufio.Reader` are shared.

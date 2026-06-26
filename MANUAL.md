# Emacs Jail MCP Manual

Emacs Jail MCP

Manages an Emacs instance running inside a copy-on-write disposable sandbox. Allows LLMs to control sandbox container, run elisp code, and capture logs and screenshots.

```text
emacs-jail-mcp [command] [global flags] [command flags]
```

### Commands

* [emacs-jail-mcp serve](#emacs-jail-mcp-serve)
* [emacs-jail-mcp info](#emacs-jail-mcp-info)
* [emacs-jail-mcp send](#emacs-jail-mcp-send)
* [emacs-jail-mcp send control](#emacs-jail-mcp-send-control)
* [emacs-jail-mcp send eval](#emacs-jail-mcp-send-eval)
* [emacs-jail-mcp send bytecomp](#emacs-jail-mcp-send-bytecomp)
* [emacs-jail-mcp send shell](#emacs-jail-mcp-send-shell)
* [emacs-jail-mcp send screenshot](#emacs-jail-mcp-send-screenshot)
* [emacs-jail-mcp send logs](#emacs-jail-mcp-send-logs)
* [emacs-jail-mcp send help](#emacs-jail-mcp-send-help)
* [emacs-jail-mcp man](#emacs-jail-mcp-man)
* [emacs-jail-mcp completion](#emacs-jail-mcp-completion)
* [emacs-jail-mcp completion bash](#emacs-jail-mcp-completion-bash)
* [emacs-jail-mcp completion zsh](#emacs-jail-mcp-completion-zsh)
* [emacs-jail-mcp completion fish](#emacs-jail-mcp-completion-fish)
* [emacs-jail-mcp completion powershell](#emacs-jail-mcp-completion-powershell)
* [emacs-jail-mcp completion help](#emacs-jail-mcp-completion-help)
* [emacs-jail-mcp help](#emacs-jail-mcp-help)

# Commands

## `emacs-jail-mcp serve`

Run the MCP server.

Without --stdio (default): listens on a TCP port (SSE transport) for MCP client and
CLI client connections.

With --stdio: reads MCP JSON-RPC messages from stdin/stdout (stdio transport).

```text
emacs-jail-mcp serve [flags]
```

### Command Flags

```text
      --stdio                     use stdio transport instead of TCP SSE transport
      --podman-binary string      podman binary name or path (default "podman")
      --no-sudo                   disable sudo for podman commands
      --display-width int         override auto-detected Xvfb display width in pixels
      --display-height int        override auto-detected Xvfb display height in pixels
      --display-depth int         Xvfb display color depth (default 24)
  -e, --emacs-binary string       emacs binary name or path (default "emacs")
      --emacs-socket-dir string   directory for the emacs-jail-rpc Unix socket (default "/tmp")
  -H, --mcp-host string           host for the MCP SSE server to bind to (default "127.0.0.1")
  -P, --mcp-port int              TCP port for the MCP SSE server (default 9421)
      --start-timeout duration    timeout waiting for jail to start (default 4m0s)
      --stop-timeout duration     timeout waiting for jail to stop (default 10s)
      --exec-timeout duration     timeout for elisp eval and shell commands (default 30s)
  -h, --help                      help for serve
```

## `emacs-jail-mcp info`

Check whether the MCP SSE server is listening on the configured TCP address.

```text
emacs-jail-mcp info [flags]
```

### Command Flags

```text
  -H, --mcp-host string   host of the MCP SSE server to connect to (default "127.0.0.1")
  -P, --mcp-port int      TCP port of the MCP SSE server (default 9421)
  -h, --help              help for info
```

## `emacs-jail-mcp send`

Send MCP requests from the command line.

Each subcommand corresponds to one MCP tool and connects to an already-running server via
its TCP port (SSE transport).

```text
emacs-jail-mcp send [flags]
```

### Command Flags

```text
  -H, --mcp-host string          host of the MCP SSE server to connect to (default "127.0.0.1")
  -P, --mcp-port int             TCP port of the MCP SSE server (default 9421)
      --start-timeout duration   timeout for jail start/stop operations (default 4m0s)
      --exec-timeout duration    timeout for eval, logs, bytecomp, and shell operations (default 30s)
  -h, --help                     help for send
```

## `emacs-jail-mcp send control`

Control the jail lifecycle.

Specify exactly one action flag: --start, --stop, --restart, or --status.

```text
emacs-jail-mcp send control [flags]
```

### Command Flags

```text
      --start     start the jail
      --stop      stop the jail
      --restart   restart the jail
      --status    show jail status
  -h, --help      help for control
```

## `emacs-jail-mcp send eval`

Evaluate an Emacs Lisp expression

```text
emacs-jail-mcp send eval EXPRESSION [flags]
```

### Command Flags

```text
  -h, --help   help for eval
```

## `emacs-jail-mcp send bytecomp`

Byte-compile an Elisp file and return diagnostics

```text
emacs-jail-mcp send bytecomp [flags]
```

### Command Flags

```text
  -f, --file-path string   path to the Elisp file to byte-compile
  -s, --severity string    filter by severity: error or warning
  -h, --help               help for bytecomp
```

## `emacs-jail-mcp send shell`

Run a shell command inside the jail container

```text
emacs-jail-mcp send shell COMMAND [flags]
```

### Command Flags

```text
  -h, --help   help for shell
```

## `emacs-jail-mcp send screenshot`

Capture a screenshot of the Emacs display

```text
emacs-jail-mcp send screenshot [flags]
```

### Command Flags

```text
  -o, --output string   output file path for the PNG screenshot
  -h, --help            help for screenshot
```

## `emacs-jail-mcp send logs`

Fetch Emacs log and diagnostic output

```text
emacs-jail-mcp send logs [flags]
```

### Command Flags

```text
  -s, --sources string   comma-separated log sources (messages, warnings, backtrace, compile_log, async_compile_log, init_log, stderr)
  -o, --offset int       1-based line offset within each source section
  -l, --limit int        max lines to return per source section
  -h, --help             help for logs
```

## `emacs-jail-mcp send help`

Help provides help for any command in the application.
Simply type send help [path to command] for full details.

```text
emacs-jail-mcp send help [command] [flags]
```

### Command Flags

```text
  -h, --help   help for help
```

## `emacs-jail-mcp man`

Generate manual page

```text
emacs-jail-mcp man [flags]
```

### Command Flags

```text
      --format string   output format: troff or markdown (default "troff")
  -h, --help            help for man
```

## `emacs-jail-mcp completion`

Generate the autocompletion script for emacs-jail-mcp for the specified shell.
See each sub-command's help for details on how to use the generated script.


```text
emacs-jail-mcp completion [flags]
```

### Command Flags

```text
  -h, --help   help for completion
```

## `emacs-jail-mcp completion bash`

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(emacs-jail-mcp completion bash)

To load completions for every new session, execute once:

#### Linux:

	emacs-jail-mcp completion bash > /etc/bash_completion.d/emacs-jail-mcp

#### macOS:

	emacs-jail-mcp completion bash > $(brew --prefix)/etc/bash_completion.d/emacs-jail-mcp

You will need to start a new shell for this setup to take effect.


```text
emacs-jail-mcp completion bash
```

### Command Flags

```text
      --no-descriptions   disable completion descriptions
  -h, --help              help for bash
```

## `emacs-jail-mcp completion zsh`

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(emacs-jail-mcp completion zsh)

To load completions for every new session, execute once:

#### Linux:

	emacs-jail-mcp completion zsh > "${fpath[1]}/_emacs-jail-mcp"

#### macOS:

	emacs-jail-mcp completion zsh > $(brew --prefix)/share/zsh/site-functions/_emacs-jail-mcp

You will need to start a new shell for this setup to take effect.


```text
emacs-jail-mcp completion zsh [flags]
```

### Command Flags

```text
      --no-descriptions   disable completion descriptions
  -h, --help              help for zsh
```

## `emacs-jail-mcp completion fish`

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	emacs-jail-mcp completion fish | source

To load completions for every new session, execute once:

	emacs-jail-mcp completion fish > ~/.config/fish/completions/emacs-jail-mcp.fish

You will need to start a new shell for this setup to take effect.


```text
emacs-jail-mcp completion fish [flags]
```

### Command Flags

```text
      --no-descriptions   disable completion descriptions
  -h, --help              help for fish
```

## `emacs-jail-mcp completion powershell`

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	emacs-jail-mcp completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.


```text
emacs-jail-mcp completion powershell [flags]
```

### Command Flags

```text
      --no-descriptions   disable completion descriptions
  -h, --help              help for powershell
```

## `emacs-jail-mcp completion help`

Help provides help for any command in the application.
Simply type completion help [path to command] for full details.

```text
emacs-jail-mcp completion help [command] [flags]
```

### Command Flags

```text
  -h, --help   help for help
```

## `emacs-jail-mcp help`

Help provides help for any command in the application.
Simply type emacs-jail-mcp help [path to command] for full details.

```text
emacs-jail-mcp help [command] [flags]
```

### Command Flags

```text
  -h, --help   help for help
```

# Using Pogo with AI agents

The passphrase prompt opens out-of-band. Agents never see it, and they do not collect it.

Run `pogo doctor`, then `pogo run -- <command>` with no key flags, and tell the person to watch for the prompt. If doctor exits 1, the person runs the command in their own terminal, or pipes a secret manager into `--key-stdin`. `--key-env` is a last resort: the value stays visible in `/proc` and `docker inspect`.

Do not guess, try, or search the filesystem for a passphrase. Do not suggest `printf 'PASSPHRASE' | pogo ...`, because that stores the passphrase in shell history. Read it from a secret manager or keychain CLI instead. There is no key file. `--key-file` and `--new-key-file` are rejected.

To see which variables are in the encrypted file, run `pogo keys`. Stdout is names only. A line like `[production]` is a section name, not a variable. `pogo keys --env NAME` prints the names `pogo run --env NAME` would set. Values are not printed. Do not run `pogo decrypt` just to learn the names.

To write a plaintext file, pass `-o` or use `-f` as the new file when the encrypted path ends in `.enc`:

```bash
pogo decrypt -f .env.enc -o .env
pogo decrypt .env.enc -f .env
```

Pogo refuses to overwrite an existing file unless you pass `--force`. Prefer `pogo run` so the values stay in memory.

Commands and flags: [commands.md](commands.md). Exit codes and editors: [reference.md](reference.md). Containers: [docker.md](docker.md). Threat model: [security.md](security.md).

## How Pogo asks

Use only one of `--key-env` and `--key-stdin`. If you pass neither and stdin is a terminal, Pogo asks in the terminal and waits until you type or press Ctrl-C. Set `POGO_TTY_TIMEOUT` to a number of seconds only when a terminal was allocated and nobody is there to type. Leave it unset for normal interactive use.

Confirmation (typing the passphrase twice) happens for that prompt and for the out-of-band prompts below, when a passphrase is being created. `--key-env` and `--key-stdin` are still read once.

When stdin is not a terminal, Pogo asks through a prompt that is already on the machine. It stops at the first one that can show a dialog. On macOS, an agent that owns the terminal (`OPENCODE`, `AGENT=1`, `CLAUDECODE`, `CLAUDE_CODE`, `CURSOR_AGENT`, `CODEX_SANDBOX`, `CODEX_THREAD_ID`, `GEMINI_CLI`, or `AIDER`) uses this same list, so the person gets the window instead of a prompt the agent cannot answer. `POGO_FORCE_TERMINAL=1` keeps the terminal prompt. A person at a normal terminal still types there.

1. `POGO_ASKPASS`, if set. Same contract as `SSH_ASKPASS`: the prompt text is the argument, one line on stdout is the passphrase, exit 0 submits, any other exit cancels.
2. The editor Git prompt, when `VSCODE_GIT_IPC_HANDLE` is a live socket. code-server, VS Code, and Cursor inject this into integrated terminals, including Remote SSH and code-server behind a reverse proxy. A Flatpak or Snap editor whose socket path is not visible is looked up under `XDG_RUNTIME_DIR`. Stderr says `Pogo is asking for the .env.enc passphrase in the editor (box at the top of the window).` code-server and VS Code show that full prompt as the placeholder. Cursor uses its own box, titled from a fake host such as `pogo-passphrase-for-.env.enc.invalid` (the username, if shown, is `pogo-passphrase`). That host ends in `.invalid`, so it does not resolve and it is not a Git login. If the editor accepts the request and does not answer, Pogo waits and then exits 1 with `pogo: the editor prompt got no answer within the timeout`. It does not open a second dialog. A missing socket or a refused connection still falls through. `POGO_PROMPT_TIMEOUT` is a number of seconds for this wait and for `POGO_ASKPASS`, the Herdr pane, and desktop dialogs. Unset, the wait is two minutes.
3. A Herdr pane, when `HERDR_ENV=1` and `HERDR_PANE_ID` is set. Pogo splits a pane, prints a prompt Ovis already recognizes, and reads the answer from a private socket. Ovis shows a masked Password or PIN field. The passphrase is not typed into the agent's pane. Cancel and timeout close the pane and stop. Stderr says `Asking for the passphrase with Herdr`.
4. A macOS dialog, when the person is logged into the GUI. The window is titled `Pogo Password Request`. The text names the file. `osascript` is already on the machine. No `DISPLAY` or Wayland socket is required. An SSH session does not show this window on the Mac's screen. Stderr says `Asking for the passphrase with macOS dialog` and `The window is titled "Pogo Password Request".` Cancel stops. A timeout stops and does not open another dialog. If the dialog cannot be shown, Pogo tries the next prompt.
5. A desktop askpass already installed, and only when a display is actually there. A local `DISPLAY` of `:N` or `unix:N` is used only when `/tmp/.X11-unix/XN` exists. A `WAYLAND_DISPLAY` name is used only when that socket exists under `XDG_RUNTIME_DIR`, or when it is an absolute path that exists. A TCP `DISPLAY` such as `localhost:10.0` is not checked for a local socket; it counts as a display. `MIR_SOCKET` counts when that path exists. If a local display variable is set and the socket is missing, the check line is `display set but no X server socket`. PATH is searched first (NixOS, Alpine, Void, Gentoo), then the usual libexec paths (Debian, Ubuntu, Pop!_OS, Fedora, RHEL, Arch, openSUSE). `gcr4-ssh-askpass` and `gcr-ssh-askpass` only relay to GNOME's prompter, so Pogo uses them when `GCR_SSH_ASKPASS_SOCKET` is a socket and otherwise tries the next helper (zenity, kdialog, or another askpass). An SSH session or WSL without WSLg has no display, so this step is skipped. Pogo does not install packages, call `pkexec`, use `systemd-ask-password`, or run the editor's `askpass.sh`.

`pogo doctor` prints which of those would be used and does not ask. Exit 0 means a prompt is available. Exit 1 means it is not. The editor section is separate and does not change that exit code, because `pogo run` does not open an editor. `POGO_DEBUG=1` prints the same decisions on stderr while a command runs. It never prints the passphrase.

If nothing can ask, Pogo exits 1. The first line is `pogo: no way to ask for the passphrase here`.

```text
New passphrase for .env.enc:
Confirm new passphrase for .env.enc:
```

That pair is what `POGO_ASKPASS`, the editor box, the macOS dialog, and a desktop askpass show. A mismatch exits 4 and writes nothing. Opening an existing file asks once. On a terminal, and in the Herdr pane, the last line is `Enter passphrase for key '.env.enc': `. Ovis masks that line. A new passphrase ends with `Enter new passphrase: `, and the confirmation ends with `Enter same passphrase again: `. `rekey` asks for the current passphrase, checks that it decrypts, and only then asks for the new one twice. Cursor titles those boxes from `pogo-passphrase-for-<file>.invalid`, `pogo-new-passphrase-for-<file>.invalid`, and `pogo-confirm-passphrase-for-<file>.invalid`. The username, if shown, stays `pogo-passphrase`.

`POGO_ASKPASS` and a desktop askpass that hit the timeout say `pogo: the POGO_ASKPASS prompt got no answer within the timeout`, or the helper's name. A helper that exits non-zero, including Escape, still says the prompt was cancelled.

## Editing without a terminal

An agent can still run `pogo edit` or `pogo init`. The passphrase uses the prompt above. The editor then opens where a person can see it, and Pogo waits until they save and close. Tell the person a tab will open. Do not ask them to paste values into chat. If no editor can open, Pogo exits 1, names what it checked, and does not print the decrypted file.

1. `$POGO_EDITOR`, if set. It is a command, so `code --wait` is allowed.
2. When `VSCODE_IPC_HOOK_CLI` is set (code-server, VS Code, VS Code Remote, Cursor), the editor's own CLI with `--wait`. Pogo looks next to `$VSCODE_GIT_ASKPASS_NODE` in `bin/remote-cli`, then on `PATH` for `code-server`, `code`, `cursor`, then `codium`. If `TERM_PROGRAM` names one of those, that one is tried first. Stderr says `Pogo opened .env.enc (decrypted) in the editor. Save and close the tab to continue (pogo waits up to 10 minutes; Ctrl+C to stop).`
3. `$EDITOR` or `$VISUAL` when it is a GUI editor that waits (`code --wait`, `cursor --wait`, `gedit -s`, `subl -w`) or any other command that is not a terminal editor. `vim`, `nano`, `vi`, and `emacs -nw` need a terminal, so they are skipped. A script in `EDITOR` still runs. `xdg-open` does not wait, so it is skipped.
4. On a real display, a known waiting GUI editor on `PATH`. `xdg-open` is not used.

`POGO_EDIT_TIMEOUT` is a number of seconds. `0` waits until the editor exits. Unset, a terminal waits until the editor exits. Unset with no terminal and a GUI or remote-CLI editor, Pogo waits up to 10 minutes. If the file was saved before that deadline, Pogo encrypts the saved text and says so. If the saved text is unchanged, `.env.enc` is not rewritten. The saved text must parse as an env file. Without a terminal, a syntax error exits 3 and writes nothing.

On a terminal the editor is `$POGO_EDITOR`, otherwise `$EDITOR`, otherwise `$VISUAL`, otherwise `vim`. The full editor rules are in [reference.md](reference.md#editing).

#!/usr/bin/env python3
"""Run a command in a pseudo-terminal and answer Pogo passphrase prompts.

The passphrase is written only to the child's tty, not to this process's stdout.
"""

from __future__ import annotations

import os
import select
import signal
import sys

PROMPTS = (
    b"Enter the new passphrase again to confirm: ",
    b"Enter a new passphrase (min 15 characters): ",
    b"Enter passphrase (min 15 characters): ",
    b"Enter it again to confirm: ",
    b"Enter the current passphrase: ",
)


def main() -> int:
    args = sys.argv[1:]
    if "--" not in args:
        print("usage: pty_prompt.py [--pid-file PATH] [--responses-file PATH] -- cmd...", file=sys.stderr)
        return 1
    split = args.index("--")
    pre, cmd = args[:split], args[split + 1 :]
    responses: list[str] = []
    pid_file = ""
    immediate = False
    i = 0
    while i < len(pre):
        if pre[i] == "--responses-file":
            responses.extend(open(pre[i + 1], encoding="utf-8").read().splitlines())
            i += 2
        elif pre[i] == "--pid-file":
            pid_file = pre[i + 1]
            i += 2
        elif pre[i] == "--immediate":
            immediate = True
            i += 1
        else:
            responses.append(pre[i])
            i += 1
    if not cmd:
        print("missing command", file=sys.stderr)
        return 1

    pid, fd = os.forkpty()
    if pid == 0:
        os.execvp(cmd[0], cmd)
        os._exit(127)
    if pid_file:
        with open(pid_file, "w", encoding="utf-8") as fh:
            fh.write(str(pid))

    def stop(signum, _frame):
        try:
            os.killpg(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        raise SystemExit(0)

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)

    pending = list(responses)
    buf = b""
    if immediate:
        if not pending:
            print("pty_prompt.py: --immediate needs a response", file=sys.stderr)
            os.kill(pid, signal.SIGKILL)
            return 1
        os.write(fd, (pending.pop(0) + "\n").encode())
    while True:
        try:
            ready, _, _ = select.select([fd], [], [], 0.5)
        except InterruptedError:
            continue
        if fd in ready:
            try:
                data = os.read(fd, 4096)
            except OSError:
                break
            if not data:
                break
            os.write(1, data)
            buf = (buf + data)[-8192:]
            for prompt in PROMPTS:
                if buf.endswith(prompt):
                    if not pending:
                        sys.stderr.write("pty_prompt.py: ran out of passphrase responses\n")
                        os.kill(pid, signal.SIGKILL)
                        return 1
                    os.write(fd, (pending.pop(0) + "\n").encode())
                    buf = b""
                    break
        else:
            wpid, status = os.waitpid(pid, os.WNOHANG)
            if wpid != 0:
                return exit_status(status)
    _, status = os.waitpid(pid, 0)
    return exit_status(status)


def exit_status(status: int) -> int:
    if os.WIFEXITED(status):
        return os.WEXITSTATUS(status)
    if os.WIFSIGNALED(status):
        return 128 + os.WTERMSIG(status)
    return 1


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except SystemExit:
        raise
    except Exception as exc:
        print(f"pty_prompt.py: {exc}", file=sys.stderr)
        raise SystemExit(1)

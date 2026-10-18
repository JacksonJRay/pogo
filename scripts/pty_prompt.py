#!/usr/bin/env python3
"""Run a command in a pseudo-terminal and answer Pogo prompts.

The passphrase is written only to the child's tty, not to this process's stdout.
A delete question (`Delete the original <file>? [y/N]`) is answered the same way.
`--stdin-file` feeds that file to the child's stdin through a pipe, and the
pty stays the controlling terminal, which is how `--key-stdin` plus the
delete question is tested.
"""

from __future__ import annotations

import fcntl
import os
import pty
import select
import signal
import sys
import termios

PROMPTS = (
    b"Enter the new passphrase again to confirm: ",
    b"Enter a new passphrase (min 15 characters): ",
    b"Enter a new passphrase: ",
    b"Enter passphrase (min 15 characters): ",
    b"Enter passphrase: ",
    b"Enter it again to confirm: ",
    b"Enter the current passphrase: ",
)


def prompt_ready(buf: bytes) -> bool:
    if buf.endswith(b"? [y/N] "):
        return True
    for prompt in PROMPTS:
        if buf.endswith(prompt):
            return True
    # "New passphrase for <file>: ", "Confirm new passphrase for <file>: ",
    # and "Current passphrase for <file>: ".
    if buf.endswith(b": ") and b"passphrase" in buf[-180:]:
        return True
    return False


def main() -> int:
    args = sys.argv[1:]
    if "--" not in args:
        print(
            "usage: pty_prompt.py [--pid-file PATH] [--responses-file PATH] [--stdin-file PATH] [--immediate] -- cmd...",
            file=sys.stderr,
        )
        return 1
    split = args.index("--")
    pre, cmd = args[:split], args[split + 1 :]
    responses: list[str] = []
    pid_file = ""
    stdin_file = ""
    immediate = False
    i = 0
    while i < len(pre):
        if pre[i] == "--responses-file":
            responses.extend(open(pre[i + 1], encoding="utf-8").read().splitlines())
            i += 2
        elif pre[i] == "--pid-file":
            pid_file = pre[i + 1]
            i += 2
        elif pre[i] == "--stdin-file":
            stdin_file = pre[i + 1]
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

    pid, fd = spawn(cmd, stdin_file)
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
            if prompt_ready(buf):
                if not pending:
                    sys.stderr.write("pty_prompt.py: ran out of responses\n")
                    os.kill(pid, signal.SIGKILL)
                    return 1
                os.write(fd, (pending.pop(0) + "\n").encode())
                buf = b""
        else:
            wpid, status = os.waitpid(pid, os.WNOHANG)
            if wpid != 0:
                return exit_status(status)
    _, status = os.waitpid(pid, 0)
    return exit_status(status)


def spawn(cmd: list[str], stdin_file: str) -> tuple[int, int]:
    if not stdin_file:
        pid, fd = os.forkpty()
        if pid == 0:
            os.execvp(cmd[0], cmd)
            os._exit(127)
        return pid, fd

    master, slave = pty.openpty()
    data = open(stdin_file, "rb").read()
    read_fd, write_fd = os.pipe()
    pid = os.fork()
    if pid == 0:
        os.close(master)
        os.close(write_fd)
        os.setsid()
        try:
            fcntl.ioctl(slave, termios.TIOCSCTTY, 0)
        except OSError as exc:
            sys.stderr.write(f"pty_prompt.py: TIOCSCTTY: {exc}\n")
            os._exit(1)
        os.dup2(read_fd, 0)
        os.dup2(slave, 1)
        os.dup2(slave, 2)
        os.close(read_fd)
        os.close(slave)
        os.execvp(cmd[0], cmd)
        os._exit(127)
    os.close(slave)
    os.close(read_fd)
    os.write(write_fd, data)
    os.close(write_fd)
    return pid, master


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

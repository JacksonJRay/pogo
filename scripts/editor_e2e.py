#!/usr/bin/env python3
"""Drive real editors (vim, nano, vim -n) through a pty.

Passphrases and env values here are fake and exist only for this test.
"""

from __future__ import annotations

import fcntl
import hashlib
import os
import select
import signal
import struct
import subprocess
import sys
import termios
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
POGO = os.environ.get("POGO", str(ROOT / "pogo"))
WORK = ROOT / ".editor-work"
PASS = "pogo-example-passphrase"
FAILS = 0


def say(status: str, detail: str) -> None:
    global FAILS
    print(f"{status} {detail}", flush=True)
    if status == "FAIL":
        FAILS += 1


def pass_(name: str) -> None:
    say("PASS", name)


def fail(name: str) -> None:
    say("FAIL", name)


def sha(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def base_env(extra: dict[str, str] | None = None) -> dict[str, str]:
    env = os.environ.copy()
    env.pop("EDITOR", None)
    env.pop("VISUAL", None)
    env["TERM"] = "xterm-256color"
    env["LANG"] = "C.UTF-8"
    if extra:
        env.update(extra)
    return env


def find_temps() -> list[Path]:
    found: list[Path] = []
    for root in (Path("/dev/shm"), Path("/tmp"), Path("/var/tmp")):
        if not root.is_dir():
            continue
        for directory in root.glob("pogo-*"):
            candidate = directory / "env"
            if candidate.is_file():
                found.append(candidate)
    return found


def plaintext_hits(needle: str) -> list[str]:
    raw = needle.encode()
    hits: list[str] = []
    for root in ("/tmp", "/dev/shm", "/var/tmp"):
        if not os.path.isdir(root):
            continue
        for dirpath, _dirnames, filenames in os.walk(root):
            for name in filenames:
                path = os.path.join(dirpath, name)
                try:
                    st = os.lstat(path)
                except OSError:
                    continue
                if not os.path.isfile(path) or st.st_size > 2_000_000 or st.st_size == 0:
                    continue
                try:
                    with open(path, "rb") as fh:
                        data = fh.read()
                except OSError:
                    continue
                if raw in data:
                    hits.append(path)
    return hits


def spawn(argv: list[str], env: dict[str, str]) -> tuple[int, int]:
    pid, fd = os.forkpty()
    if pid == 0:
        try:
            os.execvpe(argv[0], argv, env)
        finally:
            os._exit(127)
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    return pid, fd


def read_some(fd: int, buf: bytearray, timeout: float) -> bool:
    ready, _, _ = select.select([fd], [], [], timeout)
    if not ready:
        return False
    try:
        data = os.read(fd, 8192)
    except OSError:
        return False
    if not data:
        return False
    buf.extend(data)
    return True


def wait_temp(fd: int, pid: int, buf: bytearray, timeout: float = 25) -> Path | None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        read_some(fd, buf, 0.15)
        temps = find_temps()
        if temps:
            return temps[0]
        wpid, _status = os.waitpid(pid, os.WNOHANG)
        if wpid:
            return None
    return None


def finish(pid: int, fd: int, buf: bytearray, timeout: float = 20) -> int:
    deadline = time.time() + timeout
    status = None
    while time.time() < deadline:
        read_some(fd, buf, 0.2)
        wpid, st = os.waitpid(pid, os.WNOHANG)
        if wpid:
            status = st
            break
    if status is None:
        os.kill(pid, signal.SIGKILL)
        _, status = os.waitpid(pid, 0)
        try:
            os.close(fd)
        except OSError:
            pass
        return 127
    # Drain the pty so the child can exit its writes.
    end = time.time() + 1
    while time.time() < end and read_some(fd, buf, 0.1):
        pass
    try:
        os.close(fd)
    except OSError:
        pass
    if os.WIFEXITED(status):
        return os.WEXITSTATUS(status)
    if os.WIFSIGNALED(status):
        return 128 + os.WTERMSIG(status)
    return 1


def check_temp(path: Path, name: str) -> None:
    mode = path.stat().st_mode & 0o777
    dirmode = path.parent.stat().st_mode & 0o777
    if mode != 0o600:
        fail(f"{name} temp mode {oct(mode)} want 0600")
    elif dirmode != 0o700:
        fail(f"{name} temp dir mode {oct(dirmode)} want 0700")
    elif not str(path).startswith("/dev/shm/"):
        fail(f"{name} temp not on tmpfs: {path}")
    else:
        pass_(f"{name} temp 0600 on /dev/shm")


def gone(needles: list[str], name: str) -> None:
    time.sleep(0.2)
    leftover = find_temps()
    if leftover:
        fail(f"{name} temp still present: {leftover}")
        cleanup_orphans()
        return
    hits: list[str] = []
    for needle in needles:
        hits.extend(plaintext_hits(needle))
    if hits:
        fail(f"{name} plaintext left: {hits}")
    else:
        pass_(f"{name} temp gone and no plaintext in /tmp")


def drive(
    name: str,
    argv: list[str],
    env: dict[str, str],
    keys: bytes | None,
    sig: signal.Signals | None,
    needles: list[str],
    want: int,
) -> tuple[int, str]:
    buf = bytearray()
    pid, fd = spawn(argv, env)
    temp = wait_temp(fd, pid, buf)
    if temp is None:
        code = finish(pid, fd, buf, timeout=2)
        text = buf.decode("utf-8", "replace")
        fail(f"{name} editor did not open (exit {code}): {text[-300:]!r}")
        return code, text
    check_temp(temp, name)
    time.sleep(0.8)
    while read_some(fd, buf, 0.05):
        pass
    if keys is not None:
        os.write(fd, keys)
    if sig is not None:
        os.kill(pid, sig)
    code = finish(pid, fd, buf)
    text = buf.decode("utf-8", "replace")
    if code != want:
        fail(f"{name} exit {code} want {want}: {text[-400:]!r}")
    else:
        pass_(f"{name} exit {want}")
    gone(needles, name)
    return code, text


def pogo_env(enc: Path) -> str:
    proc = subprocess.run(
        [POGO, "run", "-f", str(enc), "--key-env", "POGO_PASSPHRASE", "--", "env"],
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if proc.returncode != 0:
        fail(f"run env for {enc.name} exit {proc.returncode}: {proc.stderr[-200:]}")
        return ""
    return proc.stdout


def convert(text: str, dest: Path) -> None:
    plain = WORK / "plain.env"
    plain.write_text(text)
    proc = subprocess.run(
        [POGO, "convert", str(plain), "-f", str(dest), "--key-env", "POGO_PASSPHRASE", "--force"],
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    plain.unlink(missing_ok=True)
    if proc.returncode != 0:
        raise SystemExit(f"convert failed: {proc.stderr}")


def cleanup_orphans() -> None:
    for root in (Path("/dev/shm"), Path("/tmp"), Path("/var/tmp")):
        if not root.is_dir():
            continue
        for directory in root.glob("pogo-*"):
            subprocess.run(["rm", "-rf", str(directory)], check=False)


def send_when(fd: int, pid: int, buf: bytearray, needle: bytes, payload: bytes, timeout: float = 8) -> bool:
    deadline = time.time() + timeout
    while time.time() < deadline:
        read_some(fd, buf, 0.1)
        if needle in buf:
            os.write(fd, payload)
            return True
        wpid, _status = os.waitpid(pid, os.WNOHANG)
        if wpid:
            return False
    return False


def drive_nano_save(enc: Path) -> tuple[int, str]:
    """Type a line, write it out, and quit. Nano's save prompt needs the Enter key after it appears."""
    name = "nano-save"
    buf = bytearray()
    pid, fd = spawn(
        [POGO, "edit", "-f", str(enc), "--key-env", "POGO_PASSPHRASE"],
        base_env({"VISUAL": "nano"}),
    )
    temp = wait_temp(fd, pid, buf)
    if temp is None:
        code = finish(pid, fd, buf, timeout=2)
        text = buf.decode("utf-8", "replace")
        fail(f"{name} editor did not open (exit {code})")
        return code, text
    check_temp(temp, name)
    time.sleep(0.6)
    os.write(fd, b"NANO_ADD=yes\r")
    if not send_when(fd, pid, buf, b"NANO_ADD", b"\x0f"):
        fail(f"{name} did not show the typed line")
    elif not send_when(fd, pid, buf, b"File Name to Write", b""):
        fail(f"{name} did not ask for a file name")
    else:
        mark = len(buf)
        # Nano's prompts are in raw mode: Enter is carriage return, not newline.
        os.write(fd, b"\r")
        deadline = time.time() + 8
        while time.time() < deadline and b"Wrote" not in buf[mark:]:
            read_some(fd, buf, 0.1)
        os.write(fd, b"\x18")
    code = finish(pid, fd, buf)
    text = buf.decode("utf-8", "replace")
    if code != 0:
        fail(f"{name} exit {code} want 0: {text[-400:]!r}")
    else:
        pass_(f"{name} exit 0")
    gone(["NANO_ADD=yes"], name)
    return code, text


def main() -> int:
    if not os.path.isfile(POGO) or not os.access(POGO, os.X_OK):
        print(f"missing binary {POGO}", file=sys.stderr)
        return 1
    if find_temps():
        cleanup_orphans()
    WORK.mkdir(exist_ok=True)
    for child in WORK.iterdir():
        if child.is_file():
            child.unlink()
    os.environ["POGO_PASSPHRASE"] = PASS

    # vim is the default when EDITOR and VISUAL are unset.
    init_enc = WORK / "init.enc"
    code, text = drive(
        "vim-init",
        [POGO, "init", "-f", str(init_enc), "--key-env", "POGO_PASSPHRASE", "--force"],
        base_env(),
        b":$put ='FROM_INIT=yes'\n:wq\n",
        None,
        ["FROM_INIT=yes"],
        0,
    )
    if code == 0 and init_enc.is_file():
        env = pogo_env(init_enc)
        if "FROM_INIT=yes" in env.splitlines():
            pass_("vim-init visible via pogo run -- env")
        else:
            fail(f"vim-init missing FROM_INIT in env: {text[-200:]!r}")
    elif code == 0:
        fail("vim-init did not create the encrypted file")

    src = "API_KEY=old\nKEEP=yes\nDROP_ME=drop-me-plaintext\n"
    edit_enc = WORK / "edit.enc"
    convert(src, edit_enc)
    before = sha(edit_enc)

    code, text = drive(
        "vim-edit",
        [POGO, "edit", "-f", str(edit_enc), "--key-env", "POGO_PASSPHRASE"],
        base_env(),
        b":%s/^API_KEY=.*/API_KEY=changed/\n:g/^DROP_ME=/d\n:$put ='NEW_VAR=added'\n:wq\n",
        None,
        ["API_KEY=changed", "DROP_ME=drop-me-plaintext", "NEW_VAR=added"],
        0,
    )
    if code == 0:
        env = pogo_env(edit_enc)
        lines = set(env.splitlines())
        if {"API_KEY=changed", "KEEP=yes", "NEW_VAR=added"} <= lines and not any(
            line.startswith("DROP_ME=") for line in lines
        ):
            pass_("vim-edit add/change/delete visible via pogo run -- env")
        else:
            fail(f"vim-edit env mismatch: {[line for line in lines if line.startswith(('API_KEY', 'KEEP', 'DROP', 'NEW'))]}")

    before = sha(edit_enc)
    code, text = drive(
        "vim-discard",
        [POGO, "edit", "-f", str(edit_enc), "--key-env", "POGO_PASSPHRASE"],
        base_env(),
        b":q!\n",
        None,
        ["API_KEY=changed", "NEW_VAR=added"],
        0,
    )
    if code == 0:
        if sha(edit_enc) != before:
            fail("vim-discard rewrote the encrypted file")
        elif "No changes" not in text:
            fail(f"vim-discard missing explanation: {text[-200:]!r}")
        else:
            env = pogo_env(edit_enc)
            if "API_KEY=changed" in env.splitlines() and "NEW_VAR=added" in env.splitlines():
                pass_("vim-discard left the file unchanged")
            else:
                fail("vim-discard changed variables")

    before = sha(edit_enc)
    code, _text = drive(
        "vim-cq",
        [POGO, "edit", "-f", str(edit_enc), "--key-env", "POGO_PASSPHRASE"],
        base_env(),
        b":cq\n",
        None,
        ["API_KEY=changed"],
        3,
    )
    if code == 3:
        if sha(edit_enc) == before:
            pass_("vim-cq left the encrypted file untouched")
        else:
            fail("vim-cq changed the encrypted file")

    before = sha(edit_enc)
    drive(
        "vim-sigint",
        [POGO, "edit", "-f", str(edit_enc), "--key-env", "POGO_PASSPHRASE"],
        base_env(),
        None,
        signal.SIGINT,
        ["API_KEY=changed"],
        1,
    )
    if sha(edit_enc) == before:
        pass_("vim-sigint left the encrypted file untouched")
    else:
        fail("vim-sigint changed the encrypted file")

    before = sha(edit_enc)
    drive(
        "vim-sigterm",
        [POGO, "edit", "-f", str(edit_enc), "--key-env", "POGO_PASSPHRASE"],
        base_env(),
        None,
        signal.SIGTERM,
        ["API_KEY=changed"],
        1,
    )
    if sha(edit_enc) == before:
        pass_("vim-sigterm left the encrypted file untouched")
    else:
        fail("vim-sigterm changed the encrypted file")

    nano_enc = WORK / "nano.enc"
    convert("KEEP=yes\n", nano_enc)
    code, text = drive_nano_save(nano_enc)
    if code == 0:
        env = pogo_env(nano_enc)
        if "NANO_ADD=yes" in env.splitlines() and "KEEP=yes" in env.splitlines():
            pass_("nano-save visible via pogo run -- env")
        else:
            fail(f"nano-save env mismatch screen={text[-300:]!r}")

    before = sha(nano_enc)
    code, text = drive(
        "nano-discard",
        [POGO, "edit", "-f", str(nano_enc), "--key-env", "POGO_PASSPHRASE"],
        base_env({"VISUAL": "nano"}),
        b"\x18n",
        None,
        ["NANO_ADD=yes"],
        0,
    )
    if code == 0 and sha(nano_enc) == before and "No changes" in text:
        pass_("nano-discard left the file unchanged")
    elif code == 0:
        fail(f"nano-discard changed the file or skipped the message: {text[-250:]!r}")

    args_enc = WORK / "args.enc"
    convert("KEEP=yes\n", args_enc)
    code, text = drive(
        "vim-n-args",
        [POGO, "edit", "-f", str(args_enc), "--key-env", "POGO_PASSPHRASE"],
        base_env({"EDITOR": "vim -n", "VISUAL": "nano"}),
        b":$put ='ARGS_OK=yes'\n:wq\n",
        None,
        ["ARGS_OK=yes"],
        0,
    )
    if code == 0:
        env = pogo_env(args_enc)
        if "ARGS_OK=yes" in env.splitlines() and "KEEP=yes" in env.splitlines():
            pass_("vim-n arguments honored over VISUAL")
        else:
            fail(f"vim-n env mismatch screen={text[-300:]!r}")

    print(f"failures: {FAILS}")
    return 1 if FAILS else 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        raise SystemExit(1)

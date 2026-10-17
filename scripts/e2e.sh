#!/usr/bin/env bash
# Compatibility and exit-code checks against the real static pogo binary.
# Passphrases here are fake and used only by this script.
set -u
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
POGO="${POGO:-$ROOT/pogo}"
CHECK="$ROOT/scripts/check_env.py"
PTY="$ROOT/scripts/pty_prompt.py"
PASS="pogo-example-passphrase"
PASS2="pogo-example-passphrase-v2"
PORT="43111"
WORK="${TMPDIR:-/tmp}/pogo-e2e-$$"
RESULTS="$ROOT/e2e-results.txt"
FAILS=0
SRV_PIDS=()

mkdir -p "$WORK"
: >"$RESULTS"

pass() { printf 'PASS %s\n' "$1" | tee -a "$RESULTS"; }
fail() { printf 'FAIL %s\n' "$*" | tee -a "$RESULTS"; FAILS=$((FAILS + 1)); }
skip() { printf 'UNTESTED %s\n' "$*" | tee -a "$RESULTS"; }

stop_servers() {
  local pid
  for pid in "${SRV_PIDS[@]:-}"; do
    [[ -n "$pid" ]] || continue
    kill -- -"$pid" 2>/dev/null || kill "$pid" 2>/dev/null || true
  done
  sleep 0.3
  for pid in "${SRV_PIDS[@]:-}"; do
    [[ -n "$pid" ]] || continue
    kill -9 -- -"$pid" 2>/dev/null || kill -9 "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  SRV_PIDS=()
  if command -v fuser >/dev/null 2>&1; then
    fuser -k "${PORT}/tcp" >/dev/null 2>&1 || true
    fuser -k 43121/tcp >/dev/null 2>&1 || true
    fuser -k 43122/tcp >/dev/null 2>&1 || true
    fuser -k 43123/tcp >/dev/null 2>&1 || true
  fi
  if command -v docker >/dev/null 2>&1; then
    sudo -E docker rm -f pogo-e2e-stop pogo-e2e-keyenv pogo-shared-node >/dev/null 2>&1 || true
    sudo -E docker compose -p pogo-shared-e2e -f "$ROOT/examples/shared-env-demo/compose.yaml" down --remove-orphans >/dev/null 2>&1 || true
    sudo -E docker compose -p pogo-e2e-host -f "$ROOT/examples/docker/compose.host.yaml" down --remove-orphans >/dev/null 2>&1 || true
    sudo -E docker compose -p pogo-e2e-it down --remove-orphans >/dev/null 2>&1 || true
    sudo -E docker compose -p pogo-e2e-up down --remove-orphans >/dev/null 2>&1 || true
    sudo -E docker compose -p pogo-e2e-pipe down --remove-orphans >/dev/null 2>&1 || true
  fi
}
trap stop_servers EXIT

expect_code() {
  local name="$1" want="$2"
  shift 2
  set +e
  "$@" >"$WORK/out" 2>"$WORK/err"
  local got=$?
  set +e
  if [[ "$got" -eq "$want" ]]; then
    pass "$name"
    return 0
  fi
  fail "$name (exit $got, want $want): $(tr '\n' ' ' <"$WORK/err" | head -c 400)"
  return 1
}

require_text() {
  local name="$1" file="$2"
  shift 2
  local missing=0 needle
  for needle in "$@"; do
    if ! grep -F -q -- "$needle" "$file"; then
      missing=1
      fail "$name missing text: $needle"
    fi
  done
  if [[ "$missing" -eq 0 ]]; then
    pass "$name"
  fi
}

start_server() {
  local log="$1"
  shift
  : >"$log"
  setsid "$@" >"$log" 2>&1 &
  SRV_PIDS+=($!)
}

wait_http() {
  local url="$1" tries="${2:-90}"
  local i
  for ((i = 0; i < tries; i++)); do
    if curl -fsS --max-time 2 "$url" -o "$WORK/http.body" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  return 1
}

http_case() {
  local name="$1" url="$2"
  shift 2
  local log="$WORK/${name}.log"
  start_server "$log" "$@"
  if wait_http "$url"; then
    if python3 "$CHECK" --http "$url"; then
      pass "$name"
    else
      fail "$name env mismatch"
    fi
  else
    fail "$name did not serve $url — $(tail -c 500 "$log" | tr '\n' ' ')"
  fi
  stop_servers
}

stdout_case() {
  local name="$1"
  shift
  set +e
  "$@" >"$WORK/stdout.txt" 2>"$WORK/stderr.txt"
  local got=$?
  set +e
  if [[ "$got" -ne 0 ]]; then
    fail "$name (exit $got): $(tr '\n' ' ' <"$WORK/stderr.txt" | head -c 400)"
    return
  fi
  if python3 "$CHECK" --file "$WORK/stdout.txt"; then
    pass "$name"
  else
    fail "$name stdout mismatch"
  fi
}

# shellcheck source=e2e_docker.sh
source "$ROOT/scripts/e2e_docker.sh"

if [[ ! -x "$POGO" ]]; then
  echo "missing binary $POGO — run make build" >&2
  exit 1
fi

export POGO_PASSPHRASE="$PASS"
export POGO_PASSPHRASE_V2="$PASS2"
python3 "$CHECK" --write-dotenv "$WORK/plain.env"
cp "$WORK/plain.env" "$WORK/plain.before"
RUN=("$POGO" run -f "$WORK/app.enc" --key-env POGO_PASSPHRASE --)

echo "== static binary =="
file_out="$(file "$POGO")"
printf '%s\n' "$file_out" | tee -a "$RESULTS"
if printf '%s' "$file_out" | grep -q 'statically linked'; then
  pass "file-static"
else
  fail "file-static: $file_out"
fi
ldd_out="$(ldd "$POGO" 2>&1 || true)"
printf '%s\n' "$ldd_out" | tee -a "$RESULTS"
if printf '%s' "$ldd_out" | grep -q 'not a dynamic executable'; then
  pass "ldd-static"
else
  fail "ldd-static: $ldd_out"
fi

echo "== help and usage =="
expect_code "no-args-exit-1" 1 "$POGO" || true
require_text "no-args-quick-start" "$WORK/out" "Quick start" "pogo convert .env" "pogo init" "pogo run -- npm start"
expect_code "global-help" 0 "$POGO" --help || true
require_text "global-help-quick-start" "$WORK/out" "Quick start" "passphrase"
expect_code "help-run" 0 "$POGO" help run || true
expect_code "run-help-flag" 0 "$POGO" run --help || true
expect_code "run-h" 0 "$POGO" run -h || true
if ! cmp -s <("$POGO" help run) <("$POGO" run --help); then
  fail "help forms differ"
else
  pass "help-forms-match"
fi
for cmd in init edit run encrypt decrypt convert rekey skill help version docker; do
  if "$POGO" "$cmd" --help >"$WORK/h" 2>"$WORK/he"; then
    n=$(grep -cE '^[[:space:]]+[0-9]+\. ' "$WORK/h" || true)
    if [[ "$n" -lt 3 ]]; then
      fail "help-$cmd examples ($n)"
    elif ! grep -q 'passphrase' "$WORK/h"; then
      fail "help-$cmd does not explain passphrase"
    else
      pass "help-$cmd"
    fi
  else
    fail "help-$cmd exit"
  fi
done
expect_code "unknown-command" 1 "$POGO" rn || true
require_text "did-you-mean-run" "$WORK/err" "Did you mean" "run"
expect_code "version" 0 "$POGO" version || true
expect_code "version-flag" 0 "$POGO" -V || true

echo "== exit codes and key supply =="
( cd "$WORK" && expect_code "missing-enc" 1 "$POGO" run --key-env POGO_PASSPHRASE -- true ) || true
require_text "missing-enc-next-step" "$WORK/err" "No .env.enc found in this folder." "pogo init" "pogo convert .env"
expect_code "short-passphrase" 4 env POGO_SHORT=short "$POGO" convert "$WORK/plain.env" -f "$WORK/short.enc" --key-env POGO_SHORT || true
expect_code "convert" 0 "$POGO" convert "$WORK/plain.env" -f "$WORK/app.enc" --key-env POGO_PASSPHRASE || true
if cmp -s "$WORK/plain.env" "$WORK/plain.before"; then
  pass "convert-leaves-plaintext"
else
  fail "convert-leaves-plaintext"
fi
require_text "convert-reminder" "$WORK/err" "rm "
expect_code "overwrite-refused" 1 "$POGO" convert "$WORK/plain.env" -f "$WORK/app.enc" --key-env POGO_PASSPHRASE || true
expect_code "conflict-f" 1 "$POGO" edit "$WORK/other.enc" -f "$WORK/app.enc" --key-env POGO_PASSPHRASE || true
expect_code "decrypt-wrong-key" 2 "$POGO" decrypt -f "$WORK/app.enc" --key-env POGO_PASSPHRASE_V2 || true
expect_code "decrypt-ok" 0 "$POGO" decrypt -f "$WORK/app.enc" --key-env POGO_PASSPHRASE || true
require_text "decrypt-warning" "$WORK/err" "Warning:" "plaintext"
if ! cmp -s "$WORK/out" "$WORK/plain.env"; then
  fail "decrypt-roundtrip"
else
  pass "decrypt-roundtrip"
fi
printf 'not a pogo file\n' >"$WORK/bad"
expect_code "bad-magic" 2 "$POGO" decrypt -f "$WORK/bad" --key-env POGO_PASSPHRASE || true

cat >"$WORK/append.sh" <<'EOF'
#!/bin/sh
printf '\nEDITED=yes\n' >> "$1"
EOF
chmod +x "$WORK/append.sh"
expect_code "edit" 0 env EDITOR="$WORK/append.sh" "$POGO" edit -f "$WORK/app.enc" --key-env POGO_PASSPHRASE || true
expect_code "editor-fail" 3 env EDITOR=false "$POGO" edit -f "$WORK/app.enc" --key-env POGO_PASSPHRASE || true
if ! "$POGO" decrypt -f "$WORK/app.enc" --key-env POGO_PASSPHRASE 2>/dev/null | grep -q '^EDITED=yes$'; then
  fail "edit-persisted"
else
  pass "edit-persisted"
fi

echo "== interactive editors =="
set +e
python3 "$ROOT/scripts/editor_e2e.py" | tee "$WORK/editor.out"
editor_ec=${PIPESTATUS[0]}
set +e
cat "$WORK/editor.out" >>"$RESULTS"
editor_fails=$(grep -c '^FAIL ' "$WORK/editor.out" || true)
if [[ "$editor_ec" -ne 0 && "$editor_fails" -eq 0 ]]; then
  fail "editor-e2e exit $editor_ec"
else
  FAILS=$((FAILS + editor_fails))
fi

expect_code "rekey" 0 "$POGO" rekey -f "$WORK/app.enc" --key-env POGO_PASSPHRASE --new-key-env POGO_PASSPHRASE_V2 || true
expect_code "rekey-old-fails" 2 "$POGO" decrypt -f "$WORK/app.enc" --key-env POGO_PASSPHRASE || true
expect_code "rekey-new-works" 0 "$POGO" decrypt -f "$WORK/app.enc" --key-env POGO_PASSPHRASE_V2 || true
expect_code "rekey-back" 0 "$POGO" rekey -f "$WORK/app.enc" --key-env POGO_PASSPHRASE_V2 --new-key-env POGO_PASSPHRASE || true

expect_code "long-file-unaffected" 0 "$POGO" decrypt -f "$WORK/app.enc" --key-env POGO_PASSPHRASE || true
if grep -q '^EDITED=yes$' "$WORK/out" && grep -q '^API_KEY=' "$WORK/out" && ! grep -q "easier to guess" "$WORK/out"; then
  pass "long-file-unaffected-bytes"
else
  fail "long-file-unaffected-bytes"
fi
expect_code "short-against-long-without-flag" 2 env POGO_SHORT=short "$POGO" decrypt -f "$WORK/app.enc" --key-env POGO_SHORT || true
require_text "short-against-long-decrypt-failed" "$WORK/err" "Decryption failed"
if grep -q "easier to guess" "$WORK/err" || grep -q "too short" "$WORK/err"; then
  fail "open path applied the creation minimum"
else
  pass "open-path-no-minimum"
fi
expect_code "short-against-long-with-flag" 2 env POGO_SHORT=short "$POGO" decrypt -f "$WORK/app.enc" --key-env POGO_SHORT --allow-short-passphrase || true
require_text "short-against-long-flag-still-fails" "$WORK/err" "Decryption failed"
if grep -q "easier to guess" "$WORK/err" || grep -q "easier to guess" "$WORK/out"; then
  fail "open path printed the short-passphrase warning"
else
  pass "open-flag-no-warning"
fi

expect_code "allow-short-empty" 4 env POGO_EMPTY= "$POGO" init -f "$WORK/empty.enc" --key-env POGO_EMPTY --allow-short-passphrase || true
require_text "allow-short-empty-message" "$WORK/err" "empty or only whitespace"
expect_code "allow-short-blank" 4 env POGO_BLANK='   ' "$POGO" init -f "$WORK/blank.enc" --key-env POGO_BLANK --allow-short-passphrase || true
require_text "allow-short-blank-message" "$WORK/err" "empty or only whitespace"
expect_code "allow-short-init" 3 env POGO_SHORT=short EDITOR=false "$POGO" init -f "$WORK/short-init.enc" --key-env POGO_SHORT --allow-short-passphrase || true
require_text "allow-short-init-warning" "$WORK/err" "short passphrases are much easier to guess"
if grep -q "easier to guess" "$WORK/out"; then
  fail "allow-short-init warning on stdout"
else
  pass "allow-short-init-stdout-clean"
fi

expect_code "allow-short-convert" 0 env POGO_SHORT=short "$POGO" convert "$WORK/plain.env" -f "$WORK/short.enc" --key-env POGO_SHORT --allow-short-passphrase || true
require_text "allow-short-convert-warning" "$WORK/err" "short passphrases are much easier to guess"
if grep -q "easier to guess" "$WORK/out"; then
  fail "allow-short-convert warning on stdout"
else
  pass "allow-short-convert-stdout-clean"
fi
expect_code "short-file-without-flag" 0 env POGO_SHORT=short "$POGO" decrypt -f "$WORK/short.enc" --key-env POGO_SHORT || true
if ! cmp -s "$WORK/out" "$WORK/plain.env"; then
  fail "short-decrypt-roundtrip"
else
  pass "short-decrypt-roundtrip"
fi
if grep -q "easier to guess" "$WORK/out" || grep -q "easier to guess" "$WORK/err"; then
  fail "open short file printed the short-passphrase warning"
else
  pass "short-decrypt-no-warning"
fi
expect_code "short-file-with-flag" 0 env POGO_SHORT=short "$POGO" decrypt -f "$WORK/short.enc" --key-env POGO_SHORT --allow-short-passphrase || true
if grep -q "easier to guess" "$WORK/out" || grep -q "easier to guess" "$WORK/err"; then
  fail "allow-short-decrypt warning on open"
else
  pass "allow-short-decrypt-flag-ignored"
fi
printf '%s\n' short | env POGO_SHORT=short "$POGO" decrypt -f "$WORK/short.enc" --key-stdin >"$WORK/out" 2>"$WORK/err"
if [[ $? -eq 0 ]] && cmp -s "$WORK/out" "$WORK/plain.env" && ! grep -q "easier to guess" "$WORK/err"; then
  pass "short-decrypt-key-stdin"
else
  fail "short-decrypt-key-stdin $(tr '\n' ' ' <"$WORK/err" | head -c 300)"
fi
set +e
env POGO_SHORT=short "$POGO" run -f "$WORK/short.enc" --key-env POGO_SHORT -- python3 -c 'print("CHILD_OK")' >"$WORK/short-run.out" 2>"$WORK/short-run.err"
short_run_ec=$?
set +e
if [[ "$short_run_ec" -eq 0 ]] && grep -qx 'CHILD_OK' "$WORK/short-run.out" && ! grep -q 'easier to guess' "$WORK/short-run.out" && ! grep -q 'easier to guess' "$WORK/short-run.err" && ! grep -q 'too short' "$WORK/short-run.err"; then
  pass "short-run-without-flag"
else
  fail "short-run-without-flag exit $short_run_ec out $(tr '\n' ' ' <"$WORK/short-run.out" | head -c 200) err $(tr '\n' ' ' <"$WORK/short-run.err" | head -c 300)"
fi
expect_code "allow-short-rekey-new-without-flag" 4 env POGO_SHORT=short POGO_TINY=tiny "$POGO" rekey -f "$WORK/short.enc" --key-env POGO_SHORT --new-key-env POGO_TINY || true
require_text "allow-short-rekey-new-too-short" "$WORK/err" "too short"
expect_code "allow-short-rekey" 0 env POGO_SHORT=short POGO_TINY=tiny "$POGO" rekey -f "$WORK/short.enc" --key-env POGO_SHORT --new-key-env POGO_TINY --allow-short-passphrase || true
require_text "allow-short-rekey-warning" "$WORK/err" "short passphrases are much easier to guess"
expect_code "allow-short-rekey-old" 2 env POGO_SHORT=short "$POGO" decrypt -f "$WORK/short.enc" --key-env POGO_SHORT || true
expect_code "allow-short-rekey-new" 0 env POGO_TINY=tiny "$POGO" decrypt -f "$WORK/short.enc" --key-env POGO_TINY || true
if ! cmp -s "$WORK/out" "$WORK/plain.env"; then
  fail "rekey-short-open-without-flag"
else
  pass "rekey-short-open-without-flag"
fi

expect_code "init-default-15" 4 env POGO_SHORT=short EDITOR=false "$POGO" init -f "$WORK/init15.enc" --key-env POGO_SHORT || true
require_text "init-default-15-message" "$WORK/err" "too short" "at least 15"
expect_code "encrypt-default-15" 4 env POGO_SHORT=short "$POGO" encrypt "$WORK/plain.env" -f "$WORK/enc15.enc" --key-env POGO_SHORT || true
require_text "encrypt-default-15-message" "$WORK/err" "too short"
expect_code "abcd-create" 0 env POGO_ABCD=abcd "$POGO" encrypt "$WORK/plain.env" -f "$WORK/abcd.enc" --key-env POGO_ABCD --allow-short-passphrase || true
require_text "abcd-create-warning" "$WORK/err" "short passphrases are much easier to guess"
expect_code "abcd-decrypt-no-flag" 0 env POGO_ABCD=abcd "$POGO" decrypt -f "$WORK/abcd.enc" --key-env POGO_ABCD || true
if ! cmp -s "$WORK/out" "$WORK/plain.env" || grep -q "easier to guess" "$WORK/err"; then
  fail "abcd-decrypt-no-flag-bytes"
else
  pass "abcd-decrypt-no-flag-bytes"
fi
printf '%s' abcd | "$POGO" decrypt -f "$WORK/abcd.enc" --key-stdin >"$WORK/out" 2>"$WORK/err"
if [[ $? -eq 0 ]] && cmp -s "$WORK/out" "$WORK/plain.env"; then
  pass "abcd-decrypt-key-stdin"
else
  fail "abcd-decrypt-key-stdin $(tr '\n' ' ' <"$WORK/err" | head -c 300)"
fi
set +e
env POGO_ABCD=abcd "$POGO" run -f "$WORK/abcd.enc" --key-env POGO_ABCD -- python3 -c 'print("ABCD_OK")' >"$WORK/abcd-run.out" 2>"$WORK/abcd-run.err"
abcd_run_ec=$?
set +e
if [[ "$abcd_run_ec" -eq 0 ]] && grep -qx 'ABCD_OK' "$WORK/abcd-run.out" && ! grep -q 'easier to guess' "$WORK/abcd-run.err"; then
  pass "abcd-run-no-flag"
else
  fail "abcd-run-no-flag exit $abcd_run_ec $(tr '\n' ' ' <"$WORK/abcd-run.err" | head -c 300)"
fi
expect_code "open-empty" 4 env POGO_EMPTY= "$POGO" decrypt -f "$WORK/abcd.enc" --key-env POGO_EMPTY || true
require_text "open-empty-message" "$WORK/err" "Passphrase is empty"
expect_code "abcd-wrong" 2 env POGO_WRONG=not-the-abcd-passphrase "$POGO" decrypt -f "$WORK/abcd.enc" --key-env POGO_WRONG || true
require_text "abcd-wrong-message" "$WORK/err" "Decryption failed"
cat >"$WORK/append-short.sh" <<'EOF'
#!/bin/sh
printf '\nEDITED=yes\n' >> "$1"
EOF
chmod +x "$WORK/append-short.sh"
expect_code "edit-short-no-flag" 0 env EDITOR="$WORK/append-short.sh" POGO_ABCD=abcd "$POGO" edit -f "$WORK/abcd.enc" --key-env POGO_ABCD || true
if grep -q "easier to guess" "$WORK/err" || grep -q "too short" "$WORK/err"; then
  fail "edit-short-no-flag warned"
else
  pass "edit-short-no-flag-quiet"
fi
expect_code "edit-short-roundtrip" 0 env POGO_ABCD=abcd "$POGO" decrypt -f "$WORK/abcd.enc" --key-env POGO_ABCD || true
if grep -q '^EDITED=yes$' "$WORK/out" && grep -q '^API_KEY=' "$WORK/out"; then
  pass "edit-short-persisted"
else
  fail "edit-short-persisted"
fi

expect_code "key-file-removed" 1 "$POGO" run -f "$WORK/app.enc" --key-file "$WORK/removed" -- true || true
require_text "key-file-removed-message" "$WORK/err" "removed on purpose" "--key-stdin"
expect_code "new-key-file-removed" 1 "$POGO" rekey -f "$WORK/app.enc" --new-key-file "$WORK/removed" || true
require_text "new-key-file-removed-message" "$WORK/err" "removed on purpose" "--new-key-env"

POGO_PASSPHRASE="$PASS" "$POGO" run -f "$WORK/app.enc" --key-env POGO_PASSPHRASE -- python3 -c 'import os,sys; sys.exit(0 if "POGO_PASSPHRASE" not in os.environ and os.environ.get("API_KEY")=="pogo_test_api_key_example" else 1)'
if [[ $? -eq 0 ]]; then pass "key-env"; else fail "key-env"; fi

printf '%s\n' "$PASS" | "$POGO" run -f "$WORK/app.enc" --key-stdin -- python3 -c 'import os,sys; data=sys.stdin.read(); sys.exit(0 if data=="" and os.environ.get("EQUALS_VALUE")=="a=b=c" else 1)'
if [[ $? -eq 0 ]]; then pass "key-stdin"; else fail "key-stdin"; fi

# --key-stdin on a terminal: prompt, echo off, one line. A hang here means it is still waiting for Ctrl-D.
set +e
timeout 20 python3 "$PTY" "$PASS" -- "$POGO" run -f "$WORK/app.enc" --key-stdin -- python3 -c 'import os,sys; sys.exit(0 if os.environ.get("EQUALS_VALUE")=="a=b=c" else 1)' >"$WORK/pty-stdin.out" 2>"$WORK/pty-stdin.err"
pty_ec=$?
set +e
if [[ "$pty_ec" -eq 0 ]] && grep -q "Enter passphrase: " "$WORK/pty-stdin.out" && ! grep -q "min 15" "$WORK/pty-stdin.out" && ! grep -q "$PASS" "$WORK/pty-stdin.out"; then
  pass "key-stdin-tty"
else
  fail "key-stdin-tty exit $pty_ec $(tr '\n' ' ' <"$WORK/pty-stdin.out" | head -c 300) $(tr '\n' ' ' <"$WORK/pty-stdin.err" | head -c 200)"
fi

set +e
timeout 120 python3 "$PTY" "short" -- "$POGO" run -f "$WORK/app.enc" --key-stdin -- true >"$WORK/pty-short.out" 2>"$WORK/pty-short.err"
pty_ec=$?
set +e
if [[ "$pty_ec" -eq 2 ]] && grep -q "Decryption failed" "$WORK/pty-short.out" && ! grep -q "too short" "$WORK/pty-short.out"; then
  pass "key-stdin-tty-short"
else
  fail "key-stdin-tty-short exit $pty_ec $(tr '\n' ' ' <"$WORK/pty-short.out" | head -c 300)"
fi

set +e
timeout 180 python3 "$PTY" "abcd" -- "$POGO" decrypt -f "$WORK/abcd.enc" >"$WORK/pty-abcd.out" 2>"$WORK/pty-abcd.err"
pty_ec=$?
set +e
if [[ "$pty_ec" -eq 0 ]] && grep -q "Enter passphrase: " "$WORK/pty-abcd.out" && ! grep -q "min 15" "$WORK/pty-abcd.out" && ! grep -q "easier to guess" "$WORK/pty-abcd.out" && grep -q '^EDITED=yes$' "$WORK/pty-abcd.out" && ! grep -q "abcd" "$WORK/pty-abcd.out"; then
  pass "abcd-tty-decrypt"
else
  fail "abcd-tty-decrypt exit $pty_ec $(tr '\n' ' ' <"$WORK/pty-abcd.out" | head -c 400)"
fi

set +e
timeout 180 python3 "$PTY" "abcd" -- "$POGO" run -f "$WORK/abcd.enc" -- python3 -c 'print("ABCD_TTY")' >"$WORK/pty-abcd-run.out" 2>"$WORK/pty-abcd-run.err"
pty_ec=$?
set +e
if [[ "$pty_ec" -eq 0 ]] && grep -qx 'ABCD_TTY' "$WORK/pty-abcd-run.out" && grep -q "Enter passphrase: " "$WORK/pty-abcd-run.out" && ! grep -q "easier to guess" "$WORK/pty-abcd-run.out"; then
  pass "abcd-tty-run"
else
  fail "abcd-tty-run exit $pty_ec $(tr '\n' ' ' <"$WORK/pty-abcd-run.out" | head -c 400)"
fi

set +e
timeout 20 python3 "$PTY" "" -- "$POGO" decrypt -f "$WORK/abcd.enc" >"$WORK/pty-empty.out" 2>"$WORK/pty-empty.err"
pty_ec=$?
set +e
if [[ "$pty_ec" -eq 4 ]] && grep -q "Passphrase is empty" "$WORK/pty-empty.out"; then
  pass "open-empty-tty"
else
  fail "open-empty-tty exit $pty_ec $(tr '\n' ' ' <"$WORK/pty-empty.out" | head -c 300)"
fi

set +e
timeout 180 python3 "$PTY" "short-pass" "short-pass" -- "$POGO" convert "$WORK/plain.env" -f "$WORK/pty-short.enc" --allow-short-passphrase >"$WORK/pty-allow.out" 2>"$WORK/pty-allow.err"
pty_ec=$?
set +e
if [[ "$pty_ec" -eq 0 ]] && grep -q "Enter passphrase: " "$WORK/pty-allow.out" && grep -q "short passphrases are much easier to guess" "$WORK/pty-allow.out" && ! grep -q "short-pass" "$WORK/pty-allow.out"; then
  pass "allow-short-pty"
else
  fail "allow-short-pty exit $pty_ec $(tr '\n' ' ' <"$WORK/pty-allow.out" | head -c 400) $(tr '\n' ' ' <"$WORK/pty-allow.err" | head -c 200)"
fi

set +e
timeout 120 python3 "$PTY" "not-the-right-passphrase" -- "$POGO" run -f "$WORK/app.enc" --key-stdin -- true >"$WORK/pty-bad.out" 2>"$WORK/pty-bad.err"
pty_ec=$?
set +e
if [[ "$pty_ec" -eq 2 ]] && grep -q "Decryption failed" "$WORK/pty-bad.out"; then
  pass "key-stdin-tty-decrypt-fail"
else
  fail "key-stdin-tty-decrypt-fail exit $pty_ec $(tr '\n' ' ' <"$WORK/pty-bad.out" | head -c 300)"
fi

python3 "$PTY" "$PASS" -- "$POGO" run -f "$WORK/app.enc" -- python3 -c 'import os,sys; sys.exit(0 if os.environ.get("SPACED_VALUE")=="hello world" else 1)' >"$WORK/pty.out" 2>"$WORK/pty.err"
if [[ $? -eq 0 ]]; then pass "interactive-run"; else fail "interactive-run $(tr '\n' ' ' <"$WORK/pty.err" | head -c 200) $(tr '\n' ' ' <"$WORK/pty.out" | head -c 200)"; fi

python3 "$PTY" "first-passphrase" "second-passphrase" -- "$POGO" init -f "$WORK/mismatch.enc" >"$WORK/mis.out" 2>"$WORK/mis.err" || true
if grep -q "did not match" "$WORK/mis.out" "$WORK/mis.err"; then
  pass "mismatch-explained"
else
  fail "mismatch-explained $(tr '\n' ' ' <"$WORK/mis.out" | head -c 240)"
fi

expect_code "child-exit-42" 42 "$POGO" run -f "$WORK/app.enc" --key-env POGO_PASSPHRASE -- sh -c 'exit 42' || true
expect_code "missing-command" 1 "$POGO" run -f "$WORK/app.enc" --key-env POGO_PASSPHRASE -- pogo-this-command-does-not-exist || true

echo "== override, sections, exec, tty =="
API_KEY="already-set" "$POGO" run -f "$WORK/app.enc" --key-env POGO_PASSPHRASE -- python3 -c 'import os,sys; sys.exit(0 if os.environ.get("API_KEY")=="pogo_test_api_key_example" else 1)'
if [[ $? -eq 0 ]]; then pass "override-default"; else fail "override-default"; fi
API_KEY="already-set" "$POGO" run -f "$WORK/app.enc" --key-env POGO_PASSPHRASE --no-override -- python3 -c 'import os,sys; sys.exit(0 if os.environ.get("API_KEY")=="already-set" else 1)'
if [[ $? -eq 0 ]]; then pass "no-override"; else fail "no-override"; fi
"$POGO" run -f "$WORK/app.enc" --key-env POGO_PASSPHRASE --env production -- python3 -c 'import os,sys; sys.exit(0 if os.environ.get("API_KEY")=="pogo_test_api_key_production" and os.environ.get("JWT_SECRET")=="pogo_test_jwt_secret_example" else 1)'
if [[ $? -eq 0 ]]; then pass "env-section"; else fail "env-section"; fi

setsid "$POGO" run -f "$WORK/app.enc" --key-env POGO_PASSPHRASE -- python3 -c 'import os,time; open("'"$WORK"'/pid","w").write(str(os.getpid())); time.sleep(30)' >"$WORK/exec.log" 2>&1 &
SRV_PIDS+=($!)
ok=0
for ((i = 0; i < 40; i++)); do
  if [[ -s "$WORK/pid" ]]; then ok=1; break; fi
  sleep 0.25
done
if [[ "$ok" -eq 1 ]]; then
  child=$(cat "$WORK/pid")
  comm=$(tr -d '\0' <"/proc/$child/comm" || true)
  if [[ "$child" == "${SRV_PIDS[0]}" && "$comm" == python* ]]; then
    pass "exec-replaces-process"
  else
    fail "exec-replaces-process pid=$child srv=${SRV_PIDS[0]} comm=$comm"
  fi
else
  fail "exec-replaces-process no pid $(tr '\n' ' ' <"$WORK/exec.log" | head -c 200)"
fi
stop_servers

python3 "$PTY" -- "$POGO" run -f "$WORK/app.enc" --key-env POGO_PASSPHRASE -- python3 -c 'import os; print("ISATTY", int(os.isatty(0)), int(os.isatty(1)))' >"$WORK/tty.out" 2>"$WORK/tty.err"
if grep -q 'ISATTY 1 1' "$WORK/tty.out"; then
  pass "tty-passthrough"
else
  fail "tty-passthrough $(tr '\n' ' ' <"$WORK/tty.out") $(tr '\n' ' ' <"$WORK/tty.err" | head -c 200)"
fi

setsid "$POGO" run -f "$WORK/app.enc" --key-env POGO_PASSPHRASE -- python3 -c 'import os,signal,time; signal.signal(signal.SIGINT, lambda *_: os._exit(0)); open("'"$WORK"'/sig","w").write("up"); time.sleep(60)' >"$WORK/sig.log" 2>&1 &
SRV_PIDS+=($!)
ok=0
for ((i = 0; i < 40; i++)); do
  if [[ -s "$WORK/sig" ]]; then ok=1; break; fi
  sleep 0.25
done
if [[ "$ok" -eq 1 ]]; then
  kill -INT -- -"${SRV_PIDS[0]}" 2>/dev/null || kill -INT "${SRV_PIDS[0]}"
  sleep 0.5
  if ! kill -0 "${SRV_PIDS[0]}" 2>/dev/null; then
    pass "signal-sigint"
  else
    fail "signal-sigint still running"
  fi
else
  fail "signal-sigint did not start"
fi
stop_servers

echo "== shared env demo =="
DEMO="$ROOT/examples/shared-env-demo"
rm -f "$DEMO/.env" "$DEMO/.env.enc"
cp "$DEMO/.env.example" "$DEMO/.env"
pushd "$DEMO" >/dev/null
expect_code "shared-convert" 0 "$POGO" convert .env --key-env POGO_PASSPHRASE || true
popd >/dev/null
if [[ -f "$DEMO/.env" ]]; then
  pass "shared-plaintext-kept-until-delete"
else
  fail "shared-plaintext-kept-until-delete"
fi
rm -f "$DEMO/.env"
if [[ -f "$DEMO/.env" ]]; then
  fail "shared-plaintext-deleted"
else
  pass "shared-plaintext-deleted"
fi
if [[ "$(head -c 4 "$DEMO/.env.enc")" == "POGO" ]] && ! grep -a -q 'pogo_test_api_key_example' "$DEMO/.env.enc"; then
  pass "shared-ciphertext"
else
  fail "shared-ciphertext"
fi
DEMO_KEYS="DATABASE_URL,API_KEY,JWT_SECRET,QUOTED_VALUE,SPACED_VALUE,EQUALS_VALUE,MULTILINE_VALUE,PYTHON_PORT,WEB_PORT,NODE_PORT"
start_server "$WORK/shared-python.log" "$POGO" run -f "$DEMO/.env.enc" --key-env POGO_PASSPHRASE -- python3 "$DEMO/python/app.py"
start_server "$WORK/shared-web.log" bash -lc "cd '$DEMO/web' && exec '$POGO' run -f '$DEMO/.env.enc' --key-env POGO_PASSPHRASE -- npm run dev"
py_ok=0
web_ok=0
if wait_http "http://127.0.0.1:43121/env" 40; then
  cp "$WORK/http.body" "$WORK/py.body"
  if python3 "$CHECK" --file "$WORK/py.body" --keys "$DEMO_KEYS" --override PYTHON_PORT=43121 --override WEB_PORT=43122 --override NODE_PORT=43123 \
    && ! grep -q 'pogo_test_api_key_example' "$WORK/py.body"; then
    py_ok=1
  fi
fi
if wait_http "http://127.0.0.1:43122/env" 90; then
  cp "$WORK/http.body" "$WORK/web.body"
  if python3 "$CHECK" --file "$WORK/web.body" --keys "$DEMO_KEYS" --override PYTHON_PORT=43121 --override WEB_PORT=43122 --override NODE_PORT=43123; then
    web_ok=1
  fi
fi
curl -fsS --max-time 5 "http://127.0.0.1:43121/" -o "$WORK/py.html" 2>/dev/null || true
curl -fsS --max-time 5 "http://127.0.0.1:43122/" -o "$WORK/web.html" 2>/dev/null || true
if [[ "$py_ok" -eq 1 ]] && grep -q 'po…le' "$WORK/py.html" && ! grep -q 'pogo_test_api_key_example' "$WORK/py.html"; then
  pass "shared-python"
else
  fail "shared-python $(tail -c 300 "$WORK/shared-python.log" | tr '\n' ' ')"
fi
if [[ "$web_ok" -eq 1 ]] && grep -q 'po…le' "$WORK/web.html" && ! grep -q 'pogo_test_api_key_example' "$WORK/web.html"; then
  pass "shared-sveltekit"
else
  fail "shared-sveltekit $(tail -c 400 "$WORK/shared-web.log" | tr '\n' ' ')"
fi
run_shared_node
stop_servers
rm -f "$DEMO/.env" "$DEMO/.env.enc" "$DEMO/pogo_pass.txt"

echo "== frameworks =="
http_case express "http://127.0.0.1:${PORT}/env" \
  bash -lc "cd '$ROOT/examples/node-express' && exec ${RUN[*]} npm start"

http_case go-binary "http://127.0.0.1:${PORT}/env" \
  bash -lc "cd '$ROOT/examples/go-web' && CGO_ENABLED=0 go build -o '$WORK/go-server' . && exec ${RUN[*]} '$WORK/go-server'"

http_case go-run "http://127.0.0.1:${PORT}/env" \
  bash -lc "cd '$ROOT/examples/go-web' && exec ${RUN[*]} go run ."

if [[ -x /tmp/pogo-bin/air ]]; then
  http_case air "http://127.0.0.1:${PORT}/env" \
    bash -lc "cd '$ROOT/examples/go-web' && PATH='/tmp/pogo-bin:'\"\$PATH\" exec ${RUN[*]} air"
else
  skip "air: air binary not installed"
fi

http_case sveltekit-dev "http://127.0.0.1:${PORT}/env" \
  bash -lc "cd '$ROOT/examples/sveltekit' && exec ${RUN[*]} npm run dev"

echo "building sveltekit (no secrets in the build environment)"
if ( cd "$ROOT/examples/sveltekit" && env -u API_KEY -u JWT_SECRET -u DATABASE_URL npm run build >"$WORK/svelte-build.log" 2>&1 ); then
  pass "sveltekit-build"
  http_case sveltekit-prod "http://127.0.0.1:${PORT}/env" \
    bash -lc "cd '$ROOT/examples/sveltekit' && exec ${RUN[*]} node build"
else
  fail "sveltekit-build $(tail -c 400 "$WORK/svelte-build.log" | tr '\n' ' ')"
  fail "sveltekit-prod skipped because build failed"
fi

http_case next-dev "http://127.0.0.1:${PORT}/api/env" \
  bash -lc "cd '$ROOT/examples/nextjs' && exec ${RUN[*]} npm run dev"
# Re-check the public title on a fresh server so the previous stop does not race.
start_server "$WORK/next-page.log" bash -lc "cd '$ROOT/examples/nextjs' && exec ${RUN[*]} npm run dev"
if wait_http "http://127.0.0.1:${PORT}/" 90 && grep -q 'pogo-next-public-title' "$WORK/http.body"; then
  pass "next-public-title"
else
  fail "next-public-title"
fi
stop_servers

http_case vite-dev "http://127.0.0.1:${PORT}/env" \
  bash -lc "cd '$ROOT/examples/vite-react' && exec ${RUN[*]} npm run dev"
start_server "$WORK/vite-client.log" bash -lc "cd '$ROOT/examples/vite-react' && exec ${RUN[*]} npm run dev"
if wait_http "http://127.0.0.1:${PORT}/src/App.jsx" 60 && grep -q 'pogo-vite-public-title' "$WORK/http.body"; then
  pass "vite-public-title"
else
  fail "vite-public-title $(head -c 200 "$WORK/http.body" | tr '\n' ' ')"
fi
stop_servers

stdout_case node-plain bash -lc "cd '$ROOT/examples/js-runners' && exec ${RUN[*]} node print-env.js"
stdout_case npm-start bash -lc "cd '$ROOT/examples/js-runners' && exec ${RUN[*]} npm run start"
stdout_case npm-dev-grandchild bash -lc "cd '$ROOT/examples/js-runners' && exec ${RUN[*]} npm run dev"
if command -v pnpm >/dev/null 2>&1; then
  stdout_case pnpm-start bash -lc "cd '$ROOT/examples/js-runners' && exec ${RUN[*]} pnpm run start"
else
  skip "pnpm: not installed"
fi
if command -v yarn >/dev/null 2>&1; then
  stdout_case yarn-start bash -lc "cd '$ROOT/examples/js-runners' && exec ${RUN[*]} yarn run start"
else
  skip "yarn: not installed"
fi
if command -v npx >/dev/null 2>&1; then
  stdout_case npx-node bash -lc "cd '$ROOT/examples/js-runners' && exec ${RUN[*]} npx --no-install -c 'node print-env.js'"
else
  skip "npx: not installed"
fi

http_case nodemon "http://127.0.0.1:${PORT}/env" \
  bash -lc "cd '$ROOT/examples/node-express' && exec ${RUN[*]} npx nodemon server.js"

http_case flask "http://127.0.0.1:${PORT}/env" \
  bash -lc "cd '$ROOT/examples/python' && exec ${RUN[*]} python3 flask_app.py"
http_case fastapi "http://127.0.0.1:${PORT}/env" \
  bash -lc "cd '$ROOT/examples/python' && exec ${RUN[*]} python3 -m uvicorn fastapi_app:app --host 127.0.0.1 --port ${PORT}"
http_case django "http://127.0.0.1:${PORT}/env" \
  bash -lc "cd '$ROOT/examples/django-demo' && exec ${RUN[*]} python3 manage.py runserver 127.0.0.1:${PORT}"

if command -v ruby >/dev/null 2>&1; then
  stdout_case ruby bash -lc "cd '$ROOT/examples/ruby' && exec ${RUN[*]} ruby show.rb"
  stdout_case ruby-shebang bash -lc "cd '$ROOT/examples/ruby' && exec ${RUN[*]} ./show.rb"
else
  skip "ruby: not installed"
fi

if command -v cargo >/dev/null 2>&1; then
  stdout_case rust-cargo bash -lc "cd '$ROOT/examples/rust-env' && exec ${RUN[*]} cargo run --quiet"
else
  skip "rust: cargo not installed"
fi

stdout_case shell-shebang bash -lc "cd '$ROOT/examples/shell' && exec ${RUN[*]} ./show.sh"

# PATH lookup: npm is not an absolute path.
if "$POGO" run -f "$WORK/app.enc" --key-env POGO_PASSPHRASE -- npm --version >"$WORK/npmv" 2>"$WORK/npmv.err"; then
  pass "path-lookup-npm"
else
  fail "path-lookup-npm $(tr '\n' ' ' <"$WORK/npmv.err" | head -c 200)"
fi

run_docker_matrix

echo "== summary =="
echo "failures: $FAILS"
grep -E '^(PASS|FAIL|UNTESTED) ' "$RESULTS" | tee -a "$RESULTS" >/dev/null
if [[ "$FAILS" -ne 0 ]]; then
  exit 1
fi

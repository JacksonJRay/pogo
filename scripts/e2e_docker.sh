# Docker cases for scripts/e2e.sh. Sourced, not executed.
# Expects pass, fail, skip, stdout_case, expect_code, WORK, ROOT, POGO, CHECK, PASS, RUN, DEMO, PTY.

run_shared_node() {
  if ! command -v docker >/dev/null 2>&1 || ! sudo -E docker info >/dev/null 2>&1; then
    skip "shared-node: docker daemon is not usable"
    return
  fi
  if ! sudo -E docker compose version >/dev/null 2>&1; then
    skip "shared-node: docker compose is not installed"
    return
  fi
  sudo -E docker rm -f pogo-shared-node >/dev/null 2>&1 || true
  local built=0
  if sudo -E docker compose -p pogo-shared-e2e -f "$DEMO/compose.yaml" build >"$WORK/shared-node-build.log" 2>&1; then
    built=1
  elif grep -q 'parent snapshot' "$WORK/shared-node-build.log"; then
    sudo -E docker builder prune -af >/dev/null 2>&1 || true
    if sudo -E docker compose -p pogo-shared-e2e -f "$DEMO/compose.yaml" build >"$WORK/shared-node-build.log" 2>&1; then
      built=1
    fi
  fi
  if [[ "$built" -ne 1 ]]; then
    fail "shared-node build $(tail -c 500 "$WORK/shared-node-build.log" | tr '\n' ' ')"
    return
  fi
  # Recipe: read is covered elsewhere. Here the same --key-stdin pipe starts the demo container.
  printf '%s' "$PASS" | sudo -E docker compose -p pogo-shared-e2e -f "$DEMO/compose.yaml" run -T --service-ports --rm --name pogo-shared-node node >"$WORK/shared-node.log" 2>&1 &
  local comm
  if wait_http "http://127.0.0.1:43123/env" 90; then
    cp "$WORK/http.body" "$WORK/node.body"
    comm="$(sudo -E docker exec pogo-shared-node cat /proc/1/comm 2>/dev/null | tr -d '\n' || true)"
    if python3 "$CHECK" --file "$WORK/node.body" --keys "$DEMO_KEYS" \
      --override PYTHON_PORT=43121 --override WEB_PORT=43122 --override NODE_PORT=43123 \
      && curl -fsS --max-time 5 "http://127.0.0.1:43123/" -o "$WORK/node.html" \
      && grep -q 'po…le' "$WORK/node.html" \
      && ! grep -q 'pogo_test_api_key_example' "$WORK/node.html" \
      && [[ "$comm" == "node" ]]; then
      pass "shared-node"
    else
      fail "shared-node env or pid1 comm=$comm $(tail -c 300 "$WORK/node.body" | tr '\n' ' ')"
    fi
  else
    fail "shared-node did not serve — $(tail -c 400 "$WORK/shared-node.log" | tr '\n' ' ')"
  fi
  sudo -E docker rm -f pogo-shared-node >/dev/null 2>&1 || true
}

docker_build_once() {
  local name="$1" file="$2" tag="$3" log="$4"
  sudo -E docker build -f "$file" -t "$tag" "$ROOT" >"$log" 2>&1
}

docker_build() {
  local name="$1" file="$2" tag="$3"
  local log="$WORK/docker-build-$name.log"
  if docker_build_once "$name" "$file" "$tag" "$log"; then
    pass "docker-build-$name"
    return 0
  fi
  if grep -q 'parent snapshot' "$log"; then
    sudo -E docker builder prune -af >/dev/null 2>&1 || true
    if docker_build_once "$name" "$file" "$tag" "$log"; then
      pass "docker-build-$name"
      return 0
    fi
  fi
  fail "docker-build-$name $(tail -c 400 "$log" | tr '\n' ' ')"
  return 1
}

# Pipe a passphrase into an overridden --key-stdin entrypoint. No key file.
docker_stdin_case() {
  local name="$1" tag="$2" enc="$3"
  shift 3
  set +e
  printf '%s' "$PASS" | sudo -E docker run -i --rm \
    --entrypoint /usr/local/bin/pogo \
    -v "$enc:/app/.env.enc:ro" \
    "$tag" run -f /app/.env.enc --key-stdin -- "$@" >"$WORK/docker-ep.out" 2>"$WORK/docker-ep.err"
  local got=$?
  set +e
  if [[ "$got" -ne 0 ]]; then
    fail "$name (exit $got): $(tail -c 300 "$WORK/docker-ep.err" | tr '\n' ' ') $(tail -c 200 "$WORK/docker-ep.out" | tr '\n' ' ')"
    return
  fi
  if python3 "$CHECK" --file "$WORK/docker-ep.out" \
    && grep -q 'pid=1' "$WORK/docker-ep.out" \
    && grep -q 'comm=pid1' "$WORK/docker-ep.out" \
    && grep -q 'POGO_PASSPHRASE_present=no' "$WORK/docker-ep.out" \
    && grep -q 'POGO_KEY_present=no' "$WORK/docker-ep.out"; then
    pass "$name"
  else
    fail "$name stdout $(tr '\n' ' ' <"$WORK/docker-ep.out" | head -c 300)"
  fi
}

write_compose() {
  local dest="$1" entry="$2"
  cat >"$dest" <<EOF
services:
  app:
    image: pogo-entrypoint-alpine:e2e
    stdin_open: true
    tty: true
    volumes:
      - ${WORK}/app.enc:/app/.env.enc:ro
EOF
  if [[ "$entry" == "stdin" ]]; then
    cat >"$dest" <<EOF
services:
  app:
    image: pogo-entrypoint-alpine:e2e
    entrypoint: ["pogo", "run", "-f", "/app/.env.enc", "--key-stdin", "--"]
    command: ["/app/pid1"]
    volumes:
      - ${WORK}/app.enc:/app/.env.enc:ro
EOF
  fi
}

run_docker_matrix() {
  echo "== docker =="
  if ! command -v docker >/dev/null 2>&1 || ! sudo -E docker info >/dev/null 2>&1; then
    skip "docker: docker CLI or daemon is not usable (sudo -E docker info failed)"
    return
  fi

  stdout_case docker-run bash -lc "exec ${RUN[*]} sudo -E docker run --rm \
    -e PORT -e HOST -e DATABASE_URL -e API_KEY -e JWT_SECRET \
    -e QUOTED_VALUE -e SPACED_VALUE -e EQUALS_VALUE -e MULTILINE_VALUE \
    -e VITE_PUBLIC_TITLE -e NEXT_PUBLIC_TITLE \
    -v '$ROOT/scripts/docker_env.sh:/env.sh:ro' \
    alpine:3.20 sh /env.sh"

  set +e
  "${RUN[@]}" sudo docker run --rm -e API_KEY alpine:3.20 sh -c 'printf %s "$API_KEY"' >"$WORK/docker-stripped.out" 2>"$WORK/docker-stripped.err"
  local stripped=$?
  set +e
  if [[ "$stripped" -eq 0 ]] && ! grep -q 'pogo_test_api_key_example' "$WORK/docker-stripped.out"; then
    pass "docker-sudo-drops-env"
  else
    fail "docker-sudo-drops-env exit=$stripped out=$(tr '\n' ' ' <"$WORK/docker-stripped.out" | head -c 120)"
  fi

  if ! sudo -E docker compose version >/dev/null 2>&1; then
    skip "docker-compose-host: docker compose is not installed"
  else
    sudo -E docker compose -p pogo-e2e-host -f "$ROOT/examples/docker/compose.host.yaml" down --remove-orphans >/dev/null 2>&1 || true
    set +e
    # up -d, then wait. --abort-on-container-exit can SIGTERM the short services before they flush.
    "${RUN[@]}" sudo -E docker compose -p pogo-e2e-host -f "$ROOT/examples/docker/compose.host.yaml" \
      up -d >"$WORK/compose-host.out" 2>"$WORK/compose-host.err"
    local cup=$?
    set +e
    local i cid
    for ((i = 0; i < 40; i++)); do
      local still=0
      for cid in $(sudo -E docker compose -p pogo-e2e-host -f "$ROOT/examples/docker/compose.host.yaml" ps -q 2>/dev/null); do
        if sudo -E docker inspect -f '{{.State.Running}}' "$cid" 2>/dev/null | grep -q true; then
          still=1
        fi
      done
      [[ "$still" -eq 0 ]] && break
      sleep 0.25
    done
    local passed_id title_id omit_id
    passed_id="$(sudo -E docker compose -p pogo-e2e-host -f "$ROOT/examples/docker/compose.host.yaml" ps -aq passed 2>/dev/null || true)"
    title_id="$(sudo -E docker compose -p pogo-e2e-host -f "$ROOT/examples/docker/compose.host.yaml" ps -aq interpolated 2>/dev/null || true)"
    omit_id="$(sudo -E docker compose -p pogo-e2e-host -f "$ROOT/examples/docker/compose.host.yaml" ps -aq omitted 2>/dev/null || true)"
    sudo -E docker logs "$passed_id" >"$WORK/compose-passed.out" 2>>"$WORK/compose-host.err" || true
    sudo -E docker logs "$title_id" >"$WORK/compose-title.out" 2>>"$WORK/compose-host.err" || true
    sudo -E docker logs "$omit_id" >"$WORK/compose-omit.out" 2>>"$WORK/compose-host.err" || true
    sudo -E docker compose -p pogo-e2e-host -f "$ROOT/examples/docker/compose.host.yaml" down --remove-orphans >/dev/null 2>&1 || true
    if [[ "$cup" -eq 0 ]] \
      && python3 "$CHECK" --file "$WORK/compose-passed.out" \
      && grep -q 'hello world' "$WORK/compose-title.out" \
      && grep -q 'JWT_ABSENT' "$WORK/compose-omit.out"; then
      pass "docker-compose-host"
    else
      fail "docker-compose-host exit=$cup title=$(tr '\n' ' ' <"$WORK/compose-title.out" | head -c 80) omit=$(tr '\n' ' ' <"$WORK/compose-omit.out" | head -c 80) err=$(tail -c 300 "$WORK/compose-host.err" | tr '\n' ' ')"
    fi
  fi

  if ! ( cd "$ROOT" && CGO_ENABLED=0 go build -ldflags="-s -w" -o "$ROOT/examples/docker/pid1/pid1" ./examples/docker/pid1 ); then
    fail "docker-pid1-build"
    return
  fi
  pass "docker-pid1-build"

  cp "$WORK/app.enc" "$WORK/app.readable.enc"
  chmod 644 "$WORK/app.readable.enc"

  if ! docker_build alpine "$ROOT/examples/docker/alpine.Dockerfile" pogo-entrypoint-alpine:e2e; then
    return
  fi

  # 1. Interactive docker run -it. The image entrypoint prompts.
  set +e
  python3 "$PTY" "$PASS" -- sudo -E docker run -it --rm \
    -v "$WORK/app.enc:/app/.env.enc:ro" \
    pogo-entrypoint-alpine:e2e >"$WORK/recipe-it.out" 2>"$WORK/recipe-it.err"
  local it=$?
  set +e
  if [[ "$it" -eq 0 ]] && python3 "$CHECK" --file "$WORK/recipe-it.out" \
    && grep -q 'pid=1' "$WORK/recipe-it.out" \
    && grep -q 'Enter passphrase' "$WORK/recipe-it.out"; then
    pass "docker-recipe-it"
  else
    fail "docker-recipe-it exit=$it $(tail -c 300 "$WORK/recipe-it.err" | tr '\n' ' ') $(tr '\n' ' ' <"$WORK/recipe-it.out" | head -c 200)"
  fi

  # 2. read -rs, then pipe, then unset. Driven through a pty because read has no prompt.
  cat >"$WORK/read-recipe.sh" <<EOF
#!/bin/bash
read -rs POGO_PASS && printf '%s' "\$POGO_PASS" | sudo -E docker run -i --rm \\
  --entrypoint /usr/local/bin/pogo \\
  -v "$WORK/app.enc:/app/.env.enc:ro" \\
  pogo-entrypoint-alpine:e2e \\
  run -f /app/.env.enc --key-stdin -- /app/pid1
status=\$?
unset POGO_PASS
exit \$status
EOF
  chmod +x "$WORK/read-recipe.sh"
  set +e
  python3 "$PTY" --immediate "$PASS" -- "$WORK/read-recipe.sh" >"$WORK/recipe-read.out" 2>"$WORK/recipe-read.err"
  local rd=$?
  set +e
  if [[ "$rd" -eq 0 ]] && python3 "$CHECK" --file "$WORK/recipe-read.out" && grep -q 'pid=1' "$WORK/recipe-read.out"; then
    pass "docker-recipe-read"
  else
    fail "docker-recipe-read exit=$rd $(tail -c 250 "$WORK/recipe-read.err" | tr '\n' ' ') $(tr '\n' ' ' <"$WORK/recipe-read.out" | head -c 180)"
  fi

  # 3. Password-manager shape. These shims implement the documented argv and print the passphrase.
  mkdir -p "$WORK/bin"
  cat >"$WORK/bin/op" <<'EOF'
#!/bin/sh
if [ "$1" = "read" ] && [ "$2" = "op://vault/pogo/passphrase" ]; then
  printf '%s' "$POGO_PASS"
  exit 0
fi
echo "unexpected op args: $*" >&2
exit 1
EOF
  cat >"$WORK/bin/pass" <<'EOF'
#!/bin/sh
if [ "$1" = "show" ] && [ "$2" = "pogo" ]; then
  printf '%s\n' "$POGO_PASS"
  exit 0
fi
echo "unexpected pass args: $*" >&2
exit 1
EOF
  chmod +x "$WORK/bin/op" "$WORK/bin/pass"
  set +e
  POGO_PASS="$PASS" PATH="$WORK/bin:$PATH" "$WORK/bin/op" read op://vault/pogo/passphrase | sudo -E docker run -i --rm \
    --entrypoint /usr/local/bin/pogo \
    -v "$WORK/app.enc:/app/.env.enc:ro" \
    pogo-entrypoint-alpine:e2e \
    run -f /app/.env.enc --key-stdin -- /app/pid1 >"$WORK/recipe-op.out" 2>"$WORK/recipe-op.err"
  local op=$?
  set +e
  if [[ "$op" -eq 0 ]] && python3 "$CHECK" --file "$WORK/recipe-op.out"; then
    pass "docker-recipe-op"
  else
    fail "docker-recipe-op exit=$op $(tail -c 200 "$WORK/recipe-op.err" | tr '\n' ' ')"
  fi
  set +e
  POGO_PASS="$PASS" "$WORK/bin/pass" show pogo | sudo -E docker run -i --rm \
    --entrypoint /usr/local/bin/pogo \
    -v "$WORK/app.enc:/app/.env.enc:ro" \
    pogo-entrypoint-alpine:e2e \
    run -f /app/.env.enc --key-stdin -- /app/pid1 >"$WORK/recipe-pass.out" 2>"$WORK/recipe-pass.err"
  local ps=$?
  set +e
  if [[ "$ps" -eq 0 ]] && python3 "$CHECK" --file "$WORK/recipe-pass.out"; then
    pass "docker-recipe-pass"
  else
    fail "docker-recipe-pass exit=$ps $(tail -c 200 "$WORK/recipe-pass.err" | tr '\n' ' ')"
  fi

  # 4. --key-env. Value comes from the caller. docker inspect shows it. The process does not.
  sudo -E docker rm -f pogo-e2e-keyenv >/dev/null 2>&1 || true
  if POGO_KEY="$PASS" sudo -E docker run -d --name pogo-e2e-keyenv \
    -e POGO_KEY \
    -v "$WORK/app.enc:/app/.env.enc:ro" \
    --entrypoint /usr/local/bin/pogo \
    pogo-entrypoint-alpine:e2e \
    run -f /app/.env.enc --key-env POGO_KEY -- /app/pid1 >"$WORK/keyenv.id" 2>"$WORK/keyenv.err"; then
    sudo -E docker wait pogo-e2e-keyenv >"$WORK/keyenv.wait" || true
    sudo -E docker logs pogo-e2e-keyenv >"$WORK/keyenv.out" 2>"$WORK/keyenv.logs.err" || true
    sudo -E docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' pogo-e2e-keyenv >"$WORK/keyenv.inspect" || true
    sudo -E docker rm -f pogo-e2e-keyenv >/dev/null 2>&1 || true
    if python3 "$CHECK" --file "$WORK/keyenv.out" \
      && grep -q 'POGO_KEY_present=no' "$WORK/keyenv.out" \
      && grep -q "POGO_KEY=$PASS" "$WORK/keyenv.inspect" \
      && ! grep -q "$PASS" "$WORK/keyenv.out"; then
      pass "docker-recipe-key-env"
    else
      fail "docker-recipe-key-env $(tr '\n' ' ' <"$WORK/keyenv.out" | head -c 200) inspect=$(tr '\n' ' ' <"$WORK/keyenv.inspect" | head -c 120)"
    fi
  else
    fail "docker-recipe-key-env start $(tr '\n' ' ' <"$WORK/keyenv.err" | head -c 200)"
  fi

  # 5a. docker compose run allocates a terminal and Pogo prompts.
  write_compose "$WORK/compose.interactive.yaml" interactive
  sudo -E docker compose -p pogo-e2e-it -f "$WORK/compose.interactive.yaml" down --remove-orphans >/dev/null 2>&1 || true
  set +e
  python3 "$PTY" "$PASS" -- sudo -E docker compose -p pogo-e2e-it -f "$WORK/compose.interactive.yaml" run --rm app \
    >"$WORK/recipe-crun.out" 2>"$WORK/recipe-crun.err"
  local cr=$?
  set +e
  sudo -E docker compose -p pogo-e2e-it -f "$WORK/compose.interactive.yaml" down --remove-orphans >/dev/null 2>&1 || true
  if [[ "$cr" -eq 0 ]] && python3 "$CHECK" --file "$WORK/recipe-crun.out" && grep -q 'Enter passphrase' "$WORK/recipe-crun.out"; then
    pass "docker-recipe-compose-run"
  else
    fail "docker-recipe-compose-run exit=$cr $(tail -c 250 "$WORK/recipe-crun.err" | tr '\n' ' ') $(tr '\n' ' ' <"$WORK/recipe-crun.out" | head -c 160)"
  fi

  # 5b. compose up with stdin_open and tty, then docker attach.
  sudo -E docker compose -p pogo-e2e-up -f "$WORK/compose.interactive.yaml" down --remove-orphans >/dev/null 2>&1 || true
  if sudo -E docker compose -p pogo-e2e-up -f "$WORK/compose.interactive.yaml" up -d >"$WORK/recipe-up.err" 2>&1; then
    local cid i ready=0
    cid="$(sudo -E docker compose -p pogo-e2e-up -f "$WORK/compose.interactive.yaml" ps -q app)"
    for ((i = 0; i < 30; i++)); do
      if [[ -n "$cid" ]] && sudo -E docker inspect -f '{{.State.Running}}' "$cid" 2>/dev/null | grep -q true; then
        ready=1
        break
      fi
      sleep 0.2
      cid="$(sudo -E docker compose -p pogo-e2e-up -f "$WORK/compose.interactive.yaml" ps -q app)"
    done
    set +e
    python3 "$PTY" --immediate "$PASS" -- sudo -E docker attach --sig-proxy=false "$cid" >"$WORK/recipe-attach.out" 2>"$WORK/recipe-attach.err"
    local at=$?
    set +e
    sudo -E docker compose -p pogo-e2e-up -f "$WORK/compose.interactive.yaml" down --remove-orphans >/dev/null 2>&1 || true
    if [[ "$ready" -eq 1 && "$at" -eq 0 ]] && python3 "$CHECK" --file "$WORK/recipe-attach.out" && grep -q 'pid=1' "$WORK/recipe-attach.out"; then
      pass "docker-recipe-compose-attach"
    else
      fail "docker-recipe-compose-attach ready=$ready exit=$at $(tail -c 200 "$WORK/recipe-attach.err" | tr '\n' ' ') $(tr '\n' ' ' <"$WORK/recipe-attach.out" | head -c 160)"
    fi
  else
    fail "docker-recipe-compose-attach up $(tail -c 200 "$WORK/recipe-up.err" | tr '\n' ' ')"
  fi

  # 5c. pipe into docker compose run -T.
  write_compose "$WORK/compose.stdin.yaml" stdin
  set +e
  printf '%s' "$PASS" | sudo -E docker compose -p pogo-e2e-pipe -f "$WORK/compose.stdin.yaml" run -T --rm app \
    >"$WORK/recipe-pipe.out" 2>"$WORK/recipe-pipe.err"
  local pipe=$?
  set +e
  sudo -E docker compose -p pogo-e2e-pipe -f "$WORK/compose.stdin.yaml" down --remove-orphans >/dev/null 2>&1 || true
  if [[ "$pipe" -eq 0 ]] && python3 "$CHECK" --file "$WORK/recipe-pipe.out" && grep -q 'pid=1' "$WORK/recipe-pipe.out"; then
    pass "docker-recipe-compose-pipe"
  else
    fail "docker-recipe-compose-pipe exit=$pipe $(tail -c 250 "$WORK/recipe-pipe.err" | tr '\n' ' ')"
  fi

  # No TTY and no key flag: the error names -it and --key-stdin.
  set +e
  sudo -E docker run --rm \
    -v "$WORK/app.enc:/app/.env.enc:ro" \
    pogo-entrypoint-alpine:e2e >"$WORK/recipe-notty.out" 2>"$WORK/recipe-notty.err"
  local nt=$?
  set +e
  if [[ "$nt" -eq 1 ]] && grep -q 'not a terminal' "$WORK/recipe-notty.err" \
    && grep -q -- '-it' "$WORK/recipe-notty.err" && grep -q -- '--key-stdin' "$WORK/recipe-notty.err"; then
    pass "docker-recipe-notty"
  else
    fail "docker-recipe-notty exit=$nt $(tr '\n' ' ' <"$WORK/recipe-notty.err" | head -c 300)"
  fi

  expect_code "docker-exit-42" 42 bash -lc "printf '%s' '$PASS' | sudo -E docker run -i --rm \
    --entrypoint /usr/local/bin/pogo \
    -v '$WORK/app.enc:/app/.env.enc:ro' \
    pogo-entrypoint-alpine:e2e \
    run -f /app/.env.enc --key-stdin -- /app/pid1 exit42" || true

  sudo -E docker rm -f pogo-e2e-stop >/dev/null 2>&1 || true
  if POGO_KEY="$PASS" sudo -E docker run -d --name pogo-e2e-stop \
    -e POGO_KEY \
    -v "$WORK/app.enc:/app/.env.enc:ro" \
    --entrypoint /usr/local/bin/pogo \
    pogo-entrypoint-alpine:e2e \
    run -f /app/.env.enc --key-env POGO_KEY -- /app/pid1 waitterm >"$WORK/stop.id" 2>"$WORK/stop.err"; then
    local i ready=0
    for ((i = 0; i < 40; i++)); do
      if sudo -E docker logs pogo-e2e-stop 2>&1 | grep -q 'pid=1'; then
        ready=1
        break
      fi
      sleep 0.25
    done
    local top
    top="$(sudo -E docker top pogo-e2e-stop 2>/dev/null || true)"
    sudo -E docker stop -t 5 pogo-e2e-stop >"$WORK/stop.out" 2>>"$WORK/stop.err" || true
    local code logs
    code="$(sudo -E docker inspect -f '{{.State.ExitCode}}' pogo-e2e-stop 2>/dev/null || echo missing)"
    logs="$(sudo -E docker logs pogo-e2e-stop 2>&1 || true)"
    sudo -E docker rm -f pogo-e2e-stop >/dev/null 2>&1 || true
    if [[ "$ready" -eq 1 && "$code" == "0" ]] && printf '%s' "$logs" | grep -q 'got-signal' \
      && printf '%s' "$logs" | grep -q 'comm=pid1' \
      && printf '%s' "$top" | grep -q 'pid1' \
      && ! printf '%s' "$top" | grep -q 'pogo run'; then
      pass "docker-stop-pid1"
    else
      fail "docker-stop-pid1 ready=$ready code=$code top=$(printf '%s' "$top" | tr '\n' ' ' | head -c 200) logs=$(printf '%s' "$logs" | tr '\n' ' ' | head -c 200)"
    fi
  else
    fail "docker-stop-pid1 start $(tr '\n' ' ' <"$WORK/stop.err" | head -c 200)"
  fi

  if docker_build scratch "$ROOT/examples/docker/scratch.Dockerfile" pogo-entrypoint-scratch:e2e; then
    docker_stdin_case docker-entrypoint-scratch pogo-entrypoint-scratch:e2e "$WORK/app.enc" /app/pid1
  fi

  if timeout 90 sudo -E docker pull gcr.io/distroless/static-debian12:nonroot >"$WORK/distroless-pull.log" 2>&1; then
    if docker_build distroless "$ROOT/examples/docker/distroless.Dockerfile" pogo-entrypoint-distroless:e2e; then
      docker_stdin_case docker-entrypoint-distroless pogo-entrypoint-distroless:e2e "$WORK/app.readable.enc" /app/pid1
    fi
  else
    skip "docker-entrypoint-distroless: could not pull gcr.io/distroless/static-debian12:nonroot ($(tail -c 200 "$WORK/distroless-pull.log" | tr '\n' ' '))"
  fi

  if docker_build pogo-image "$ROOT/examples/docker/pogo.Dockerfile" pogo:e2e; then
    set +e
    sudo -E docker run --rm pogo:e2e version >"$WORK/pogo-image.out" 2>"$WORK/pogo-image.err"
    local pv=$?
    set +e
    if [[ "$pv" -eq 0 ]] && grep -q 'pogo ' "$WORK/pogo-image.out"; then
      pass "docker-pogo-image"
    else
      fail "docker-pogo-image exit=$pv $(tr '\n' ' ' <"$WORK/pogo-image.err" | head -c 200)"
    fi
  fi

  if timeout 120 sudo -E docker pull golang:1.22-alpine >"$WORK/golang-pull.log" 2>&1; then
    if docker_build pogo-src "$ROOT/examples/docker/pogo-src.Dockerfile" pogo:src-e2e; then
      set +e
      sudo -E docker run --rm pogo:src-e2e version >"$WORK/pogo-src.out" 2>"$WORK/pogo-src.err"
      local sv=$?
      set +e
      if [[ "$sv" -eq 0 ]] && grep -q 'pogo ' "$WORK/pogo-src.out"; then
        pass "docker-pogo-src"
      else
        fail "docker-pogo-src exit=$sv $(tr '\n' ' ' <"$WORK/pogo-src.err" | head -c 200)"
      fi
    fi
  else
    skip "docker-pogo-src: could not pull golang:1.22-alpine ($(tail -c 200 "$WORK/golang-pull.log" | tr '\n' ' '))"
  fi
}

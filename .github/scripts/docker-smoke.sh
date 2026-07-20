#!/bin/sh
# Verify the shipped image end to end on a real Docker engine: build it, serve
# a fixture repo both ways the entrypoint supports, and assert the board
# behaves. CI runs this; run it by hand to reproduce anywhere Docker exists:
#
#   .github/scripts/docker-smoke.sh [image-tag]
#
# The Dockerfile was written and reviewed without a Docker engine on the
# author's machine, so until this ran, "docker build works" was an assumption.
set -eu

IMAGE="${1:-truthboard:smoke}"
WORK=$(mktemp -d)
CONTAINERS=""

say() { printf '\n== %s\n' "$*"; }
ok() { printf '  ✓ %s\n' "$*"; }
fail() {
    printf 'docker-smoke: %s\n' "$*" >&2
    for c in $CONTAINERS; do
        printf '\n--- docker logs %s ---\n' "$c" >&2
        docker logs "$c" >&2 2>&1 || true
    done
    exit 1
}

cleanup() {
    for c in $CONTAINERS; do
        docker rm -f "$c" >/dev/null 2>&1 || true
    done
    rm -rf "$WORK"
}
trap cleanup EXIT

start() { # name, then docker run args — records the name for logs and cleanup
    name=$1
    shift
    CONTAINERS="$CONTAINERS $name"
    docker run -d --name "$name" "$@" "$IMAGE" >/dev/null
}

wait_ready() { # base-url, container
    i=0
    while [ "$i" -lt 60 ]; do
        if curl -fsS -o /dev/null "$1/" 2>/dev/null; then
            ok "board answering at $1"
            return 0
        fi
        docker inspect -f '{{.State.Running}}' "$2" 2>/dev/null | grep -q true ||
            fail "$2 exited before serving"
        i=$((i + 1))
        sleep 1
    done
    fail "$2 never served $1"
}

expect_status() { # url want label [curl args...]
    url=$1 want=$2 label=$3
    shift 3
    got=$(curl -s -o /dev/null -w '%{http_code}' "$@" "$url")
    [ "$got" = "$want" ] || fail "$label: HTTP $got, want $want"
    ok "$label → $got"
}

# ---------------------------------------------------------------- fixture ---
say "fixture repository"
FIX="$WORK/fixture"
mkdir -p "$FIX/.truthboard/specs"
git init -q -b main "$FIX"
git -C "$FIX" config user.email smoke@example.com
git -C "$FIX" config user.name "Docker Smoke"
printf 'console.log("hi")\n' >"$FIX/app.js"
cat >"$FIX/.truthboard/specs/tb-5m0k-serve-a-board.md" <<'SPEC'
---
id: tb-5m0k
title: Serve a board from a container
---

## Goal

Give the containerised board a story to derive a status for.

## Acceptance

- [ ] The board renders this spec
SPEC
git -C "$FIX" add -A
git -C "$FIX" commit -qm "Initial commit"
# The board clones from here in the REPO_URL variant; a bare repo is what a
# real remote looks like, and gives the sync loop something to fetch from.
git clone -q --bare "$FIX" "$WORK/fixture.git"
# The container runs as an unprivileged user that is not the host's.
chmod -R a+rwX "$WORK"
ok "fixture built"

# ------------------------------------------------------------------ build ---
say "docker build"
docker build -t "$IMAGE" --build-arg VERSION=smoke . || fail "docker build failed"
ok "image $IMAGE built"

# --------------------------------------------- variant 1: REPO_URL clone ---
say "REPO_URL clone, default port, polling sync"
start board-url -p 1337:1337 -v "$WORK/fixture.git:/src.git:ro" -e REPO_URL=file:///src.git
wait_ready http://127.0.0.1:1337 board-url

expect_status http://127.0.0.1:1337/ 200 "page"
expect_status http://127.0.0.1:1337/api/board 200 "board api"

# Beyond loopback the board is shared, and a shared board without an edit
# token serves read-only — proof is never writable over the network.
expect_status http://127.0.0.1:1337/api/specs 403 "write refused" \
    -X POST -H 'Content-Type: application/json' -d '{"title":"nope"}'

version=$(curl -s -D - -o /dev/null http://127.0.0.1:1337/api/board | tr -d '\r' |
    sed -n 's/^[Xx]-[Tt]ruthboard-[Vv]ersion: //p')
[ "$version" = "deliberately-wrong" ] || fail "version header = '$version', want the VERSION build arg 'smoke'"
ok "version header carries the build arg"

# The sync loop reports freshness once it has fetched; poll rather than assume
# a fetch has already happened.
i=0
while [ "$i" -lt 40 ]; do
    headers=$(curl -s -D - -o /dev/null http://127.0.0.1:1337/api/board | tr -d '\r')
    if printf '%s' "$headers" | grep -qi '^x-truthboard-sync-'; then
        ok "sync headers present: $(printf '%s' "$headers" | grep -ic '^x-truthboard-sync-') of them"
        break
    fi
    i=$((i + 1))
    sleep 1
done
[ "$i" -lt 40 ] || fail "no X-Truthboard-Sync-* header after 40s"

curl -s http://127.0.0.1:1337/api/board | grep -q 'tb-5m0k' ||
    fail "the board does not carry the fixture's spec"
ok "fixture spec present in the board"

# ------------------------------- variant 2: mounted /repo, PORT, FETCH=0 ---
say "mounted /repo, custom PORT, webhook-only (FETCH=0)"
start board-mount -p 8080:8080 -v "$FIX:/repo" -e PORT=8080 -e FETCH=0
wait_ready http://127.0.0.1:8080 board-mount

expect_status http://127.0.0.1:8080/ 200 "page on PORT=8080"
expect_status http://127.0.0.1:8080/api/board 200 "board api on PORT=8080"
expect_status http://127.0.0.1:8080/api/specs 403 "write refused" \
    -X POST -H 'Content-Type: application/json' -d '{"title":"nope"}'
ok "mounted clone served without cloning"

# ---------------------------------------- variant 3: no repo at all fails ---
say "no repository provided"
docker run --rm --name board-norepo "$IMAGE" >"$WORK/norepo.log" 2>&1 && {
    cat "$WORK/norepo.log" >&2
    fail "a container with no repo must exit non-zero"
}
grep -q "mount a clone there or set REPO_URL" "$WORK/norepo.log" ||
    fail "missing the actionable no-repo message: $(cat "$WORK/norepo.log")"
ok "refuses with the actionable message"

say "all docker checks passed"

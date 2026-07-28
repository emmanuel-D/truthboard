#!/bin/sh
# Serve a shared board for the repo at $REPO_DIR. Two ways to provide it:
# mount a clone (-v /path/to/clone:/repo), or set REPO_URL and the
# container clones it on first start. Extra arguments are passed through
# to `truthboard ui`.
set -e

REPO_DIR="${REPO_DIR:-/repo}"

if [ ! -e "$REPO_DIR/.git" ] && [ -n "$REPO_URL" ]; then
    # Prove the credential works before cloning with it. A platform that
    # restarts crashed containers turns a bad token into the same git error
    # printed once per restart; this turns it into one diagnosis instead.
    truthboard preflight --remote "$REPO_URL" || exit 1
    # Deploy platforms archive their logs and the documented way to reach a
    # private repo is a token in the URL, so strip it before printing.
    echo "cloning $(printf '%s' "$REPO_URL" | sed -E 's#://[^@/]*@#://***@#') into $REPO_DIR"
    git clone "$REPO_URL" "$REPO_DIR"
fi

if [ ! -e "$REPO_DIR/.git" ]; then
    echo "no git repository at $REPO_DIR — mount a clone there or set REPO_URL" >&2
    exit 1
fi

# TRUTHBOARD_WEBHOOK_SECRET and TRUTHBOARD_NOTIFY_URL are read by the
# binary itself; FETCH=0 disables polling for webhook-only boards.
exec truthboard ui \
    --host "${HOST:-0.0.0.0}" \
    --port "${PORT:-1337}" \
    --fetch "${FETCH:-60s}" \
    --no-open \
    "$@" \
    "$REPO_DIR"

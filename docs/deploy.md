# Deploy a shared board

A Truthboard board is a single static binary reading a git clone — so a
shared board is just that binary running on any machine your team can
reach: an EC2 instance, a VPS, a Coolify app. Everyone gets one URL,
every device with a browser sees the live board, and nobody has to ask
"what's the status?" in chat.

**The one rule:** a board served beyond loopback is **read-only by
default**. The shared board shows the truth and edits nothing — creating
and editing stories stays a same-machine privilege, from a clone (web UI
on localhost, `truthboard spec new`, or an agent over MCP). Since
statuses are derived from git anyway, the shared board loses nothing on
the proof side. To open intent editing on a shared board — write a story
from your phone, have your agent pick it up at home — arm an
[edit token](#editing-from-anywhere-the-edit-token).

Whatever the shape, the server needs exactly three things:

1. **the `truthboard` binary** (or the Docker image, which contains it),
2. **a clone of your repo** — the board derives everything from it,
3. **a way to stay fresh** — `--fetch` polling, or the push webhook.

## Option A — plain binary + systemd (EC2, any VPS)

Install the binary and clone the repo:

```sh
# releases ship tarballs named truthboard_<tag>_<os>_<arch>.tar.gz;
# resolve the current tag so this line never goes stale
TAG=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
  https://github.com/emmanuel-D/truthboard/releases/latest | sed 's#.*/##')
curl -fsSL "https://github.com/emmanuel-D/truthboard/releases/download/${TAG}/truthboard_${TAG}_linux_amd64.tar.gz" \
  | tar -xz -C /usr/local/bin truthboard

git clone https://github.com/you/your-project /srv/your-project
```

(While the truthboard repo is private, unauthenticated downloads 404 —
use `gh release download --repo emmanuel-D/truthboard --pattern
'truthboard_*_linux_amd64.tar.gz'` instead. `truthboard update` keeps
the binary current afterwards either way.)

For a private repo, clone over SSH with a read-only deploy key, or over
HTTPS with a fine-grained token in the remote URL — the board only ever
fetches, so read scope is enough.

Then a systemd unit, `/etc/systemd/system/truthboard.service`:

```ini
[Unit]
Description=Truthboard shared board
After=network-online.target

[Service]
User=truthboard
ExecStart=/usr/local/bin/truthboard ui --host 0.0.0.0 --port 1337 \
    --fetch 60s --no-open /srv/your-project
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```sh
systemctl enable --now truthboard
```

The unit keeps the board in the foreground under systemd's supervision —
no `--detach` needed (that flag is for laptops, where nothing else
supervises the process).

## Option B — Docker

The repo ships a `Dockerfile`. Build once:

```sh
docker build -t truthboard .
```

CI builds this image on every push and exercises it against a fixture
repository — both start-up forms, the `PORT` and `FETCH` knobs, reads
answering `200`, writes refused with `403`, sync headers present, and the
no-repository case failing with its message. Reproduce that run anywhere
Docker exists:

```sh
.github/scripts/docker-smoke.sh
```

Run it either by letting the container clone your repo:

```sh
docker run -d -p 1337:1337 \
  -e REPO_URL=https://github.com/you/your-project \
  truthboard
```

or by mounting a clone you manage yourself:

```sh
docker run -d -p 1337:1337 -v /srv/your-project:/repo truthboard
```

The container serves on `0.0.0.0:1337` and polls origin every 60s by
default. Environment knobs:

| Variable | Default | Meaning |
| --- | --- | --- |
| `REPO_URL` | — | clone this repo into `/repo` on first start |
| `PORT` | `1337` | listen port |
| `FETCH` | `60s` | origin poll interval; `0` disables polling (webhook-only) |
| `TRUTHBOARD_WEBHOOK_SECRET` | — | arm `POST /webhook` for push-triggered refresh |
| `TRUTHBOARD_NOTIFY_URL` | — | post stalled/regressed transitions to this webhook |
| `TRUTHBOARD_EDIT_TOKEN` | — | arm token-gated intent editing (see below) |

Extra arguments after the image name are passed through to
`truthboard ui` (e.g. `docker run … truthboard --forge`).

For a private repo with `REPO_URL`, embed a read-only token
(`https://x-access-token:<token>@github.com/you/your-project`) — or
prefer the mounted-clone form and keep credentials out of the container.
The clone (mounted or not) must be writable: fetching updates `.git`.

## Option C — Coolify

Coolify makes this the easiest of the three — it builds the Dockerfile,
wires the domain, and terminates TLS for you:

1. **Create a new application** from the Truthboard git repository and
   pick **Dockerfile** as the build pack.
2. **Set environment variables:** `REPO_URL` pointing at the repo you
   want the board for (with a read-only token if private), and
   optionally `FETCH`, `TRUTHBOARD_WEBHOOK_SECRET`,
   `TRUTHBOARD_NOTIFY_URL`.
3. **Set the port** to `1337` (Coolify's "Ports Exposes" field) and
   attach your domain. Coolify's Traefik proxy handles the board's
   server-sent events without extra configuration.
4. Deploy. The container clones the repo on first start and the board is
   live on your domain — from any device.

Note that `REPO_URL` is cloned into the container's filesystem, so a
redeploy re-clones from scratch. That is fine (the clone is disposable —
all state derives from origin); attach a persistent volume at `/repo`
only if you want faster restarts on a big repo.

## Keeping the board fresh: poll or push

Two ways, combinable:

- **Polling** — `--fetch 60s` (or the `FETCH` env in Docker): the board
  fetches origin on an interval. Zero setup beyond the flag; the board
  is at most one interval behind.
- **Push webhook** — set `--webhook-secret` (env
  `TRUTHBOARD_WEBHOOK_SECRET`) and point a GitHub (HMAC signature) or
  GitLab (`X-Gitlab-Token`) push webhook at
  `https://board.example.com/webhook`. A push triggers an immediate
  fetch + re-derive, and open browsers update instantly over
  server-sent events. Bad or missing secrets are rejected in constant
  time; the endpoint can only make the board fresher, never change what
  it says.

Webhook-only (`FETCH=0` + secret) is the quietest setup: no polling
traffic, updates the moment work lands.

## Reverse proxy and TLS

Coolify handles this for you. On a bare server, put Caddy or nginx in
front for TLS. The one caveat: the live-update stream at `/api/events`
is server-sent events, which buffering proxies break. Caddy passes SSE
through by default; for nginx, disable buffering on the board location:

```nginx
location / {
    proxy_pass http://127.0.0.1:1337;
    proxy_buffering off;
    proxy_set_header Connection '';
    proxy_http_version 1.1;
}
```

If SSE is blocked anyway, the board still works — the page falls back to
its normal refresh cycle; updates are just not instant.

## Editing from anywhere: the edit token

By default a shared board is read-only. Arm it with an edit token —
`--edit-token <secret>` or `TRUTHBOARD_EDIT_TOKEN` (in Docker/Coolify,
just set the env var) — and intent editing opens up, gated on that
token:

- On the board, tap **🔑 unlock** and paste the token once — it is
  remembered in that browser. Reading never needs it; only writes carry
  it.
- Every story you create or edit is **committed to the server's clone
  and pushed to origin** by the board itself, with a clear
  `Intent: <title> (<id>) — from the shared board` message. If origin
  moved in the meantime, the board rebases your edit on top; a real
  conflict backs out cleanly and tells you to resolve from a clone.
- A push failure (dead credentials, network) is shown on the page, not
  buried in a server log — the edit is still committed on the server's
  clone, so nothing is lost.

**The round trip this buys you:** on the road, open the board on your
phone, write a story or file a bug. The board pushes it to origin. Back
at your PC, `git pull` (or your agent's own fetch) brings it in, and
`truthboard next` — or an agent calling `next_spec` over MCP — picks it
up as the next thing to build. Idea to agent, zero copy-paste.

Two deployment notes:

- The server clone now needs **push** access: a read-write deploy key,
  or a token with write scope in `REPO_URL`
  (`https://x-access-token:<token>@github.com/you/your-project`).
- Statuses are still derived. The token opens the promise (spec files),
  never the proof — there remains no route by which a status could be
  written, from anywhere.

Treat the edit token like the webhook secret: serve the board over TLS
only, and rotate the token by restarting with a new value.

## What the shared board can and cannot do

Can: everything read — the kanban board, drift, digest, sprint state,
card detail, filters; live updates; stalled/regressed notifications via
`--notify`. With an edit token: create and edit stories, landed on
origin by the board itself.

Cannot (without a token): create or edit stories — writes get a `403`
explaining that intent editing needs a clone. And with or without one:
statuses could never be written from anywhere; there is no route by
which one could be set.

# Deploy a shared board

A Truthboard board is a single static binary reading a git clone — so a
shared board is just that binary running on any machine your team can
reach: an EC2 instance, a VPS, a Coolify app. Everyone gets one URL,
every device with a browser sees the live board, and nobody has to ask
"what's the status?" in chat.

**The one rule:** a board served beyond loopback is strictly
**read-only**. There is no auth story yet, so the shared board shows the
truth and edits nothing — creating and editing stories stays a
same-machine privilege, from a clone (web UI on localhost, `truthboard
spec new`, or an agent over MCP). Since statuses are derived from git
anyway, the shared board loses nothing on the proof side; only intent
editing stays local.

Whatever the shape, the server needs exactly three things:

1. **the `truthboard` binary** (or the Docker image, which contains it),
2. **a clone of your repo** — the board derives everything from it,
3. **a way to stay fresh** — `--fetch` polling, or the push webhook.

## Option A — plain binary + systemd (EC2, any VPS)

Install the binary and clone the repo:

```sh
# releases ship tarballs named truthboard_<tag>_<os>_<arch>.tar.gz
curl -fsSL https://github.com/emmanuel-D/truthboard/releases/latest/download/truthboard_v0.5.0_linux_amd64.tar.gz \
  | tar -xz -C /usr/local/bin truthboard

git clone https://github.com/you/your-project /srv/your-project
```

(Check [Releases](https://github.com/emmanuel-D/truthboard/releases) for
the current tag; `truthboard update` keeps it current afterwards.)

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

## What the shared board can and cannot do

Can: everything read — the kanban board, drift, digest, sprint state,
card detail, filters; live updates; stalled/regressed notifications via
`--notify`.

Cannot: create or edit stories — writes get a `403` explaining that
intent editing needs a clone. Statuses could never be written from
anywhere; there is no route by which one could be set. Remote intent
editing behind an edit token is planned (`tb-6e13`).

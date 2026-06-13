# fetchanddeploy

English | [日本語](README-ja.md)

A single-binary tool that receives GitHub push events via webhook, pulls the target repository, and runs arbitrary deploy commands. Intended for automated deployment on a VPS and similar setups.

- Runs as a single Go binary — easy to distribute
- Handles multiple repositories in one process
- Verifies signatures with `X-Hub-Signature-256` (HMAC-SHA256)
- Per-repository mutex prevents overlapping deploys
- Responds with `202` immediately to avoid GitHub's 10-second timeout, then deploys asynchronously

## Build

```sh
go build -o fetchanddeploy .
# Embed a version:
go build -ldflags "-X main.version=$(git describe --tags --always)" -o fetchanddeploy .

# Cross-compile for a Linux VPS (from macOS):
GOOS=linux GOARCH=amd64 go build -o fetchanddeploy .
```

## Configuration

Copy `config.example.yaml` and edit it.

```yaml
listen: ":9000"
path: "/webhook"
repositories:
  - name: "owner/repo"          # GitHub full name
    branch: "main"               # branch to react to (empty = all branches)
    path: "/var/www/app"         # working directory
    secret: "your webhook secret"
    timeout: 5m
    deploy:
      - "git fetch --all --prune"
      - "git reset --hard origin/main"
      - "docker compose up -d --build"
```

The following environment variables are available inside deploy commands: `FAD_REPO` / `FAD_BRANCH` / `FAD_COMMIT`.

## Running

```sh
./fetchanddeploy -config /etc/fetchanddeploy/config.yaml
```

Use `-version` to print the version. `/healthz` is available for health checks.

## GitHub setup

In the repository, go to **Settings → Webhooks → Add webhook**:

- Payload URL: `https://your-domain/webhook`
- Content type: `application/json`
- Secret: the same value as `secret` in the config file
- Events: **Just the push event**

## Reverse proxy (nginx example)

This tool serves HTTP only. Terminate TLS at the proxy.

```nginx
location /webhook {
    proxy_pass http://127.0.0.1:9000/webhook;
    proxy_set_header Host $host;
}
```

## systemd

Place `fetchanddeploy.service` in `/etc/systemd/system/`.

```sh
sudo cp fetchanddeploy /usr/local/bin/
sudo mkdir -p /etc/fetchanddeploy
sudo cp config.example.yaml /etc/fetchanddeploy/config.yaml  # then edit it
sudo cp fetchanddeploy.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now fetchanddeploy
sudo journalctl -u fetchanddeploy -f
```

## Notes

- The target `path` must already contain a git clone of the repository (this tool does not clone — it only runs the configured deploy commands).
- If you pull over SSH, provide the SSH key / known_hosts for the service user.
- Deploy commands run via `/bin/sh -c`, so treat the config file carefully (it allows arbitrary command execution).

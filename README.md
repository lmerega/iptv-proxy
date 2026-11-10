# 📡 IPTV Proxy — lmerega fork

A self-hosted reverse proxy for IPTV M3U playlists and Xtream Codes API. Expose your IPTV provider behind your own credentials and hostname, with optional VPN routing via Gluetun.

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/license-GPL--3.0-green)
![Docker](https://img.shields.io/badge/docker-ghcr.io%2Flmerega%2Fiptv--proxy-blue?logo=docker)
[![Tests and container builds](https://github.com/lmerega/iptv-proxy/actions/workflows/docker-image.yml/badge.svg)](https://github.com/lmerega/iptv-proxy/actions/workflows/docker-image.yml)


## Origin and maintenance

**This is an independently maintained fork of [WireshJ/iptv-proxy](https://github.com/WireshJ/iptv-proxy), not an original project by lmerega.** The original application was created by [Pierre-Emmanuel Jacquier](https://github.com/pierre-emmanuelJ/iptv-proxy), with subsequent work by [incmve](https://github.com/incmve/iptv-proxy) and [WireshJ](https://github.com/WireshJ/iptv-proxy). Their work, copyright notices and license are retained. See [Credits](#-credits) for the contributions inherited from each project.

[lmerega](https://github.com/lmerega) maintains the changes, documentation and container builds published **in this fork**. This repository does not imply endorsement by the earlier authors, ownership of their work, or an official transfer of upstream maintenance. Contributions and future upstream collaboration are welcome.

### Why this fork exists

We needed fixes for Xtream catch-up playback and providers returning numeric stream category IDs. These were submitted upstream as [PR #1](https://github.com/WireshJ/iptv-proxy/pull/1) and [PR #2](https://github.com/WireshJ/iptv-proxy/pull/2). We maintain and distribute them here so users can use the fixes independently of the upstream review schedule.

**Activity checked on 10 November 2026:** the latest commit on the upstream default branch was dated **11 May 2026**, and both PRs remained open without reviews or comments. Upstream appears inactive based on those observations; its author has not announced abandonment or transferred maintainership to us. This dated note is not a claim about future upstream activity.

### What changes here

- Preserve the provider's Xtream EPG envelope and encoded fields.
- Support catch-up query endpoints and normalize supported AM/PM timestamps.
- Correct the advertised Xtream hostname and narrowly repair malformed byte-range advertisements.
- Accept both numeric and quoted stream category IDs, avoiding catalog decoding failures.
- Run regression tests, `go vet` and a build before publishing our own multi-platform containers.

See [FORK_CHANGES.md](FORK_CHANGES.md) for dated changes and links to the patches. This remains a proxy: catch-up availability and retained history depend on your provider. It does not add a local DVR.

**Use this fork's image:** [`ghcr.io/lmerega/iptv-proxy`](https://github.com/lmerega/iptv-proxy/pkgs/container/iptv-proxy), for `linux/amd64` and `linux/arm64`. The upstream image will not receive our independent changes automatically. See [Installation](#-installation), [Migration and updates](#migration-and-updates), and [License and source](#license-and-source).

---

## Table of Contents

- [Origin and maintenance](#origin-and-maintenance)
- [Features](#-features)
- [Requirements](#-requirements)
- [Installation](#-installation)
  - [Docker Compose without VPN (quick start)](#docker-compose-without-vpn-quick-start)
  - [Docker (recommended)](#docker-recommended)
  - [Docker Compose with Gluetun VPN](#docker-compose-with-gluetun-vpn)
  - [Manual (Go binary)](#manual-go-binary)
- [Migration and updates](#migration-and-updates)
- [Configuration](#-configuration)
  - [Environment variables](#environment-variables)
- [Usage](#-usage)
  - [M3U proxy](#m3u-proxy)
  - [Xtream Codes proxy](#xtream-codes-proxy)
- [Support and contributing](#support-and-contributing)
- [License and source](#license-and-source)
- [Credits](#-credits)

---

## ✨ Features

| Feature | Description |
|---------|-------------|
| 🔄 **M3U proxy** | Rewrites all channel URLs in your M3U playlist to point to the proxy |
| 📺 **M3U8 proxy** | Transparent proxy for HLS (m3u8) streams |
| 🎬 **Xtream Codes** | Full proxy for the Xtream Codes client API — live, VOD, series, EPG |
| 🔐 **Credential swap** | Replaces provider credentials with your own user/password |
| 🌐 **VPN support** | Route all traffic through Gluetun (or any network namespace) |
| ⚡ **Caching** | M3U playlist cached locally to reduce provider requests |

---

## 📋 Requirements

| Requirement | Notes |
|-------------|-------|
| Docker | For the recommended container setup |
| Go 1.25+ | Only for manual/binary builds |
| IPTV provider | M3U URL or Xtream Codes credentials |

---

## 🚀 Installation

### Docker Compose without VPN (quick start)

Install Docker with the Compose plugin, then download the standalone example:

```bash
mkdir iptv-proxy && cd iptv-proxy
curl -fLO https://raw.githubusercontent.com/lmerega/iptv-proxy/master/compose.standalone.yaml
curl -fsSL https://raw.githubusercontent.com/lmerega/iptv-proxy/master/.env.example -o .env
```

Edit `.env` before starting. Set `HOSTNAME` to the hostname or IP **reachable by your player**, without a scheme or path. `XTREAM_BASE_URL` includes the provider's scheme and port. `PROXY_USER` / `PROXY_PASSWORD` are the new credentials your players will use; `XTREAM_USER` / `XTREAM_PASSWORD` are your existing provider credentials. Choose your own proxy credentials, and keep `.env` private. Quote values containing spaces, `#` or `$` appropriately for Compose's `.env` format (single quotes preserve literal `$`).

```bash
docker compose -f compose.standalone.yaml config --quiet
docker compose -f compose.standalone.yaml pull
docker compose -f compose.standalone.yaml up -d
docker compose -f compose.standalone.yaml ps
```

In your Xtream-compatible player, use `http://<HOSTNAME>:8080`, `PROXY_USER` and `PROXY_PASSWORD`. Provider credentials stay in the server configuration. To use a different port, set `PORT` and `ADVERTISED_PORT` consistently. Configure TLS separately with your reverse proxy if needed; `HTTPS=true` controls generated URLs and does not install a TLS listener.

This example does not use a VPN. For M3U-only providers, use the Docker command below with `M3U_URL`. For Gluetun, use the separate VPN example.

### Docker (recommended)

```bash
docker run -d \
  --name iptv-proxy \
  -p 8080:8080 \
  -e HOSTNAME=your-server-ip \
  -e PORT=8080 \
  -e M3U_URL="http://provider.example.com:8000/get.php?username=user&password=pass&type=m3u_plus&output=mpegts" \
  -e USER=myuser \
  -e PASSWORD=mypassword \
  -e GIN_MODE=release \
  ghcr.io/lmerega/iptv-proxy:latest
```

**Port:** `8080`

---

### Docker Compose with Gluetun VPN

The existing [docker-compose.yml](docker-compose.yml) is a separate Gluetun example. Fill in your VPN/provider credentials and adapt the VPN provider settings to your account before running it:

```bash
mkdir -p /data/apps/iptv-proxy
curl -o /data/apps/iptv-proxy/docker-compose.yml \
  https://raw.githubusercontent.com/lmerega/iptv-proxy/master/docker-compose.yml

cd /data/apps/iptv-proxy
# Edit docker-compose.yml with your VPN and IPTV credentials
docker compose up -d
```

> The `iptv-proxy` container shares the Gluetun network namespace — all IPTV traffic is routed through the VPN automatically.

**Volumes:**
- `/data/apps/gluetun` — Gluetun configuration and state
- `/data/apps/iptv-proxy` — optional local M3U file mount

---

### Manual (Go binary)

```bash
git clone https://github.com/lmerega/iptv-proxy.git
cd iptv-proxy

go build -mod=vendor -o iptv-proxy .

./iptv-proxy \
  --hostname your-server-ip \
  --port 8080 \
  --m3u-url "http://provider.example.com/iptv.m3u" \
  --user myuser \
  --password mypassword
```

All CLI flags can also be set as environment variables (replace `-` with `_`, e.g. `M3U_URL`).

---

## Migration and updates

### Choose an image version

| Reference | Behavior |
|-----------|----------|
| `ghcr.io/lmerega/iptv-proxy:latest` | Moves when a successful build of `master` is published |
| `ghcr.io/lmerega/iptv-proxy:sha-<full-40-character-commit>` | Identifies the source commit; available from successful builds |
| `ghcr.io/lmerega/iptv-proxy@sha256:<digest>` | Pins the exact published image manifest |

For a fixed deployment, copy a published commit tag or digest from the [package page](https://github.com/lmerega/iptv-proxy/pkgs/container/iptv-proxy). Place it in `IPTV_PROXY_IMAGE` for the standalone example, or in your existing service's `image:` field. Do not use the angle-bracket placeholders literally. Pulling an image does not update a running container; recreation is required.

### Switch an existing installation

1. Back up your Compose file and private environment files. Record the running image reference **and image ID/digest** so you can restore the same version; retain that local image until validation succeeds.
2. In the `iptv-proxy` service, change only `image:` from the upstream/local image to a published `ghcr.io/lmerega/iptv-proxy` reference. Keep credentials, ports, volumes, VPN/network mode and other settings intact. Existing player settings can remain the same when those values are unchanged.
3. Validate, pull and recreate the proxy using the correct Compose file for your installation. For the standalone example:

   ```bash
   docker compose -f compose.standalone.yaml config --quiet
   docker compose -f compose.standalone.yaml pull iptv-proxy
   docker compose -f compose.standalone.yaml up -d --no-deps iptv-proxy
   docker compose -f compose.standalone.yaml ps
   ```

4. Check login, live/VOD categories and lists, then actual live/VOD/catch-up playback in your player. Passing a catalog request does not prove playback or provider archive availability.
5. If checks fail, restore the backed-up image reference (or the recorded immutable digest) and configuration, then recreate only the proxy. Avoid `down`, volume deletion, or image pruning during migration.

**Existing shared VPN namespace:** if your service uses `network_mode: service:gluetun`, preserve the running VPN when updating only the proxy. Some Compose versions cannot resolve that service dependency with `--no-deps`. Inspect the existing proxy's network mode:

```bash
docker inspect --format '{{.HostConfig.NetworkMode}}' <existing-proxy-container>
```

If it is `container:<vpn-container-id>`, put that exact value in a temporary override:

```yaml
services:
  iptv-proxy:
    network_mode: "container:<vpn-container-id>"
```

Use your normal Compose file **plus** this override with `up -d --no-deps iptv-proxy`, then verify the VPN's container ID and start time are unchanged. Keep the original `service:gluetun` in your persistent Compose file; the temporary override is tied to the current VPN container and must be regenerated after that container is replaced. Do not recreate the VPN merely to change the proxy image.

For later updates, repeat the pull, recreation and validation steps. Pinning a commit/digest means updates are deliberate; it does not automatically track `latest`.

---

## ⚙️ Configuration

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HOSTNAME` | *(required)* | Hostname or IP that clients will connect to |
| `PORT` | `8080` | Port the proxy listens on |
| `ADVERTISED_PORT` | same as `PORT` | Port exposed to clients — useful behind a reverse proxy |
| `M3U_URL` | — | Remote M3U URL or path to local file (e.g. `/root/iptv/iptv.m3u`) |
| `M3U_FILE_NAME` | `iptv.m3u` | Filename of the proxified M3U endpoint |
| `M3U_CACHE_EXPIRATION` | `1` | M3U cache lifetime in hours |
| `USER` | `usertest` | Username clients use to authenticate with the proxy |
| `PASSWORD` | `passwordtest` | Password clients use to authenticate with the proxy |
| `XTREAM_USER` | — | Xtream Codes username from your provider |
| `XTREAM_PASSWORD` | — | Xtream Codes password from your provider |
| `XTREAM_BASE_URL` | — | Xtream Codes base URL e.g. `http://provider.example.com:8080` |
| `XTREAM_API_GET` | `false` | Generate `get.php` from the Xtream API instead of the original endpoint |
| `HTTPS` | `false` | Use HTTPS for proxified URLs |
| `CUSTOM_ENDPOINT` | — | Optional path prefix for all proxy endpoints |
| `GIN_MODE` | — | Set to `release` to suppress Gin debug output |
| `DEBUG` | — | Set to `true` to enable debug logging |

> **Xtream auto-detection:** If your `M3U_URL` contains `/get.php` with `username` and `password`, Xtream mode is enabled automatically — no need to set `XTREAM_USER`, `XTREAM_PASSWORD`, or `XTREAM_BASE_URL` separately.

---

## 🔌 Usage

### M3U proxy

After starting the proxy, your proxified M3U playlist is available at:

```
http://<HOSTNAME>:<PORT>/iptv.m3u?username=<USER>&password=<PASSWORD>
```

All channel URLs in the playlist point back to the proxy. Clients stream through the proxy to the original provider.

**Example with original M3U:**
```
#EXTM3U
#EXTINF:-1 tvg-name="Channel 1",Channel 1
http://provider.example.com:1234/user/pass/1
```

**Proxified output:**
```
#EXTM3U
#EXTINF:-1 tvg-name="Channel 1",Channel 1
http://your-server-ip:8080/<token>/myuser/mypassword/0/1
```

---

### Xtream Codes proxy

When Xtream mode is active, the proxy exposes the full Xtream Codes API on your hostname with your credentials:

| Endpoint | Description |
|----------|-------------|
| `GET /get.php` | M3U playlist |
| `GET /player_api.php` | Live, VOD, series, EPG data |
| `POST /player_api.php` | Same, for apps that POST |
| `GET /xmltv.php` | EPG XML data |
| `GET /live/<user>/<pass>/:id` | Live stream |
| `GET /movie/<user>/<pass>/:id` | VOD stream |
| `GET /series/<user>/<pass>/:id` | Series stream |

**Original credentials (provider):**
```
user:     xtream_user
password: xtream_password
base-url: http://provider.example.com:8080
```

**Proxified credentials (clients use these):**
```
user:     myuser
password: mypassword
base-url: http://your-server-ip:8080
```

---

## Support and contributing

Please open [issues](https://github.com/lmerega/iptv-proxy/issues) and pull requests **in this fork** for problems with its changes or images. Include your image tag/digest, platform, player version, expected behavior and a reproducible example. Redact provider URLs, account details and authenticated request paths from logs or screenshots. Use synthetic data where possible.

To build and check the source locally:

```bash
git clone https://github.com/lmerega/iptv-proxy.git
cd iptv-proxy
go test -mod=vendor ./...
go vet -mod=vendor ./...
go build -mod=vendor ./...
docker build -t iptv-proxy:local .
```

The Go module path still uses `github.com/WireshJ/iptv-proxy` for compatibility; the source and images of this maintained fork live under `lmerega`. The vendored Xtream library contains a deliberate category ID fix: preserve it and its tests when updating dependencies.

## License and source

The inherited code is distributed under the GNU GPL; see the unchanged [LICENSE](LICENSE) and the existing source-file copyright/license notices (which permit GPL version 3 or later). Those notices and prior authors' credits remain in place. This fork's modifications are distributed under the same terms, without a warranty; the full terms are in the license.

Source for the published containers is available in [this repository](https://github.com/lmerega/iptv-proxy), including the vendored dependencies and [Dockerfile](Dockerfile). The `sha-<commit>` image tag and `org.opencontainers.image.revision` label identify the source commit; open `https://github.com/lmerega/iptv-proxy/tree/<commit>` for that build. If you redistribute modified code or containers, preserve the notices and comply with the applicable GPL terms, including the corresponding-source requirements. Attribution alone does not replace the license terms.

## 🙏 Credits

Originally created by [Pierre-Emmanuel Jacquier](https://github.com/pierre-emmanuelJ) — [pierre-emmanuelJ/iptv-proxy](https://github.com/pierre-emmanuelJ/iptv-proxy).

Extended by [incmve](https://github.com/incmve) — [incmve/iptv-proxy](https://github.com/incmve/iptv-proxy):
- Fixed Xtream Codes EPG not loading
- Fixed Xtream Codes VOD (Shows & Movies) with incomplete provider responses
- Continue-on-error for malformed `EXTINF` entries
- Gluetun VPN integration

Direct upstream: [WireshJ/iptv-proxy](https://github.com/WireshJ/iptv-proxy), developed by [WireshJ](https://github.com/WireshJ):
- Upgraded to Go 1.25, removed all deprecated APIs (`ioutil`, `satori/go.uuid`, `mitchellh/go-homedir`)
- Context-aware HTTP streaming — streams cancel cleanly when a client disconnects
- Shared HTTP client with connection pooling for better performance
- Multi-platform Docker image (`linux/amd64`, `linux/arm64`) published to GHCR via Go cross-compilation
- Fully automated CI/CD with GitHub Actions — supports semver and CalVer tags
- Updated Traefik setup to v3 with Let's Encrypt support
- Removed dead code (`pkg/utils/`) and broken test stubs
- All dependencies updated to latest stable versions

Maintained independently here by [lmerega](https://github.com/lmerega):
- Catch-up/EPG and numeric category ID fixes described above
- Regression tests, fork documentation and GHCR container publication

Thank you to the earlier authors and dependency maintainers. The full Git history is retained; this fork does not replace their authorship.

**Powered by:**
- [gin](https://github.com/gin-gonic/gin)
- [cobra](https://github.com/spf13/cobra)
- [go.xtream-codes](https://github.com/tellytv/go.xtream-codes)
- [gluetun](https://github.com/qdm12/gluetun)

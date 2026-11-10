# IPTV Proxy fork

Maintainer: lmerega. Upstream: WireshJ/iptv-proxy. Preserve credits and GPL-3.0.

The canonical branch is master. Keep the existing Go module path for compatibility.
The vendor tree includes a deliberate go.xtream-codes Stream.CategoryID decoding fix;
do not overwrite it during dependency updates without preserving the behavior and tests.

Run go test -mod=vendor ./..., go vet -mod=vendor ./... and go build -mod=vendor ./....
Use synthetic providers and .invalid domains; never commit credentials or production logs.
GitHub Actions publishes ghcr.io/lmerega/iptv-proxy for amd64 and arm64 after checks.
Use commit tags or digests for deployments and preserve a rollback reference.
Infrastructure changes require explicit authorization and belong in private operational memory,
not this public repository. Keep all upstream attribution and date any activity observations.

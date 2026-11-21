# Changes in the lmerega fork

This is a modification record for the fork, not a replacement for the original authors' history or copyright notices. Direct upstream: [WireshJ/iptv-proxy](https://github.com/WireshJ/iptv-proxy). Earlier authors and contributions are listed in [README.md](README.md#-credits).

## 21 November 2026 — retry of transient provider errors on info requests

- `get_series_info` and `get_vod_info` now retry up to twice (after 0.7 s and 1.5 s) when the provider answers 502, 503 or 504, which some providers do intermittently on single titles. Other statuses and transport errors are not retried, and the retry stops as soon as the client request is cancelled.
- Added regression tests for the retry sequence, the statuses that are not retried and the cancellation.

## 20 November 2026 — series and VOD info passthrough

- Forwarded `get_series_info` and `get_vod_info` provider JSON unchanged instead of decoding it with the typed Xtream structs, which rejected documents with empty arrays where objects are expected (episode `info`, `audio`, `video`) or mixed numeric/quoted fields and made the proxy answer HTTP 502 with an empty body.
- Kept the 16 MiB read limit, the JSON-object check, the 400 response for a missing ID and the 502 response (without credentials in the message) for provider failures.
- Added synthetic regression tests, including a fixture proving that the typed decoder rejects the provider shape.

## 10 November 2026 — maintained fork and container publication

- Integrated our catch-up and category decoding work into master while retaining upstream history.
- Added tested multi-platform publication to ghcr.io/lmerega/iptv-proxy, with source revision labels and commit tags.
- Updated container/source references and documented independent maintenance, upstream provenance, installation, migration, rollback and licensing.
- Added a standalone Compose example and placeholder environment template.

## 7 November 2026 — numeric stream category IDs

- Removed the restrictive JSON string tag from Stream.CategoryID in the vendored go.xtream-codes library, preserving FlexInt handling of numbers and strings.
- Added regression cases for numeric, quoted, mixed and invalid category IDs in live and VOD catalogs.
- [Patch on the combined branch](https://github.com/lmerega/iptv-proxy/commit/e2e0a2689bb342547243b2793d495903278b0825); [separate upstream PR #2](https://github.com/WireshJ/iptv-proxy/pull/2).

## 10 October 2026 — Xtream EPG and catch-up compatibility

- Preserved provider EPG envelopes, extra fields and Base64 text encodings.
- Supported query-style catch-up endpoints and supported 12-hour/24-hour timestamp normalization.
- Advertised a bare Xtream hostname and repaired numeric Accept-Ranges only on confirmed partial byte responses.
- Added synthetic-provider regression tests.
- [Catch-up branch revision](https://github.com/lmerega/iptv-proxy/commit/8155b6f663a2da40c73ec611511bfcecbe23a7ea); [upstream PR #1](https://github.com/WireshJ/iptv-proxy/pull/1).

All previous upstream development remains in the Git history. See LICENSE and individual source-file notices for the applicable license terms.

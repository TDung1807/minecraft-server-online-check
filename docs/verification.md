# Verification results

Executed locally on 2026-10-03 using Go 1.27.1 on macOS arm64, Docker Desktop 29.8.0, and Caddy 2.10.2. The target Minecraft server was queried without login or a plugin.

## Passed

- `go test -race ./...`: config, bounded Minecraft protocol parsing, transport failures, state transitions, poller lifecycle, REST, SSE and limits.
- `go vet ./...` and `go mod verify`.
- Native binary build and Docker image build with pinned base-image digests.
- Docker Compose configuration validation and Caddy configuration validation.
- Non-root Docker runtime (UID/GID 10001), read-only filesystem, live status query and clean shutdown (exit code 0).
- HTTPS through a local Caddy test container using its local CA: immediate SSE snapshot, subsequent updates and heartbeat. Public certificate issuance was not tested.
- Headless Chrome/Playwright at 1440x900, 390x844 and 320x640: no JavaScript errors or horizontal overflow, server icon loaded, timestamps updated over SSE.
- Simulated continuous SSE failure: REST fallback after 30 seconds, periodic REST reads, successful SSE recovery stopping fallback.
- Delayed REST success and error after a newer SSE event: neither overwrote the current snapshot or live connection indicator.
- Real `bora.pikamc.vn:25005` queries returned valid Paper 26.3 status; player counts changed during verification. No controlled player join/leave latency measurement was performed.

## Thirty-minute soak

One poller queried a local Minecraft TCP simulator every 5 seconds with 100 simultaneous SSE clients and continuous REST traffic. The Go race detector was enabled.

| Measurement | Result |
| --- | --- |
| Measured traffic duration | 30 minutes |
| Minecraft queries | 361, including initial query |
| Successful REST requests | 33,167 |
| REST p95 | 6.324875 ms |
| Live snapshot events received | 36,101 |
| Heap allocation increase after GC | 603,152 bytes |
| Acceptance thresholds | p95 <100 ms; heap growth <8 MiB; no query amplification |

The complete soak passed. Subsequently strengthened cadence/freshness assertions passed a 20-second smoke run: 5 Minecraft queries, 378 REST requests, p95 3.66725 ms, heap growth 139,872 bytes, 500 snapshot events. Long-run query count and live snapshots also satisfied those criteria. Heap figures describe Go heap after GC, not total process RSS or race-detector overhead.

## Deployment checks remaining

Provide a real VPS and domain, deploy with the supplied Compose configuration, verify public HTTPS/SSE, configure Prometheus/Alertmanager, and measure controlled player joins/leaves against the 8-second target. Local tests do not establish VPS network latency, public TLS issuance, or notification delivery.

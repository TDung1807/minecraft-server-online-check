# SSE contract

`GET /api/v1/events` returns `text/event-stream`, `Cache-Control: no-store`, and `X-Accel-Buffering: no`.

The first frame sets `retry: 3000`. Snapshot frames contain `id: <sequence>`, `event: status`, and `data: <snapshot JSON>` followed by a blank line. The schema matches `GET /api/v1/status`. A comment frame `: heartbeat` is flushed every 15 seconds. There is no message replay: `Last-Event-ID` is intentionally ignored, and every new connection receives the latest snapshot.

Subscription registration and the initial snapshot use the same lock. The sequence increments on every completed query, even with unchanged player counts, and when freshness expires. Sequence is scoped to the running process: consumers must accept a lower sequence after reconnect/restart.

Each subscriber has a single-slot queue. A newer snapshot replaces a queued older snapshot. This is a status feed, not an audit/event log. Writes must complete within 10 seconds; a slow or disconnected consumer is removed without blocking the poller. Limits are 500 streams globally and 5 streams per IP; exceeding either returns 429 with `Retry-After: 60`.

The included frontend fetches REST first, then subscribes to `status`. After 30 seconds of continuous SSE failure it polls REST every 5 seconds, retries SSE every 30 seconds, and stops fallback polling after receiving a valid SSE snapshot. It expires current values locally after `staleAfterMs` if the website connection stops delivering updates. Backend UTC timestamps are rendered in `Asia/Ho_Chi_Minh`.

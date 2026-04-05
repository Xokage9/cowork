# Load Test Results

Date: 2026-04-04

Scenario:
- Endpoint: `GET /rooms/{roomId}/slots?date=...`
- Target: ~100 RPS
- Duration: 15s
- Workers: 10
- Environment: local (Windows), in-memory repositories, `httptest` server

Command:
```bash
go run ./loadtest/loadtest.go
```

Observed result:
- Total requests: 1463
- Success: 1459
- Success rate: 99.73%
- Effective RPS: 97.53
- p50 latency: 1.09ms
- p95 latency: 3.83ms
- p99 latency: 7.51ms

Notes:
- 4 failed requests were local socket-address reuse errors on Windows (`connectex: Only one usage...`).
- On Linux/macOS environments or with tuned socket settings, these transient local errors are typically lower.

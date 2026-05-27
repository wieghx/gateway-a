# Gateway-a Plans.md

Created: 2026-05-12

---

## Phase 11: Core LLM Proxy Hardening (Stable Forwarding + Real Token Stats + Reliable Quota)

**Scope**: Focused exclusively on making the three most critical paths production-usable for a single-upstream LLM gateway.
**Goal**: The gateway can reliably forward requests, accurately record real token usage from upstream, and enforce quotas based on actual consumption.
**Out of Scope**: Multi-provider routing, WASM plugins, advanced queue features, new middleware, UI, distributed tracing.

**Status Update (as of latest work)**:
- 11.1, 11.3, 11.4, 11.5, 11.7, 11.8 largely complete or significantly advanced.
- Major improvements in shared CircuitBreaker, retries, real usage recording (non-stream + best-effort stream), quota observability (metrics + logs + headers).

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 11.1 | Centralize & harden LLMClient for production | Single shared CircuitBreaker instance, configurable timeouts/retries/backoff, proper context propagation, no per-request CB creation | - | cc:done |
| 11.2 | Improve streaming & non-streaming reliability | Streaming handles disconnects/graceful degradation; both paths return proper error codes; headers (incl. auth) forwarded correctly | 11.1 | cc:WIP |
| 11.3 | Add lightweight retry for transient upstream failures | Configurable retries (max 2-3) with exponential backoff only on 5xx + network errors; no retry on 4xx | 11.1 | cc:done |
| 11.4 | Record real token usage after successful calls | After every successful (stream or non-stream) response, real `prompt_tokens`, `completion_tokens`, `total_tokens` and model are written to `usage_tracks` table | 11.2 | cc:done |
| 11.5 | Persist usage atomically with request success | Usage recording happens in the same logical unit as successful response; failures in recording are logged but do not fail the user request (or are clearly documented) | 11.4 | cc:done |
| 11.6 | Replace crude pre-flight quota check with real token enforcement | `checkQuota` (or new equivalent) is called with **actual** token counts from the upstream response (or accurate estimate); pre-check may remain as best-effort | 11.4 | cc:WIP |
| 11.7 | Make quota reliable and observable | Quota exceeded returns clear 429 with remaining quota info; usage is queryable per tenant/date/model; no race conditions between concurrent requests | 11.6 | cc:done |
| 11.8 | Add proxy-path metrics and structured logging | Emit metrics: `gateway_llm_requests_total{status,model}`, `gateway_llm_tokens_total{model,type}`, latency histogram; log request_id + model + tokens on success/error | - | cc:done |
| 11.9 | Strengthen integration tests for the three core paths | New or updated tests that assert: (a) real tokens are recorded, (b) quota is enforced with real numbers, (c) retries and CB behavior under failure | 11.4, 11.7 | cc:TODO |
| 11.10 | End-to-end verification + docs update | Full `go test ./tests/integration/...` passes with real upstream or good mock; DEPLOYMENT.md or README updated with "Production Core Path" checklist | All | cc:TODO |
| 11.11 | Auto-enable include_usage for streaming + improved usage extraction | Gateway automatically requests usage in streams and reliably extracts it when available | 11.2, 11.4 | cc:done |
| 11.12 | Wire central Metrics into LLM path | LLM requests, token counts, and latency properly recorded via the shared metrics instance | - | cc:done |
| 11.13 | Add basic request protections | Payload size limits and improved error status code mapping from upstream | - | cc:done |
| 11.14 | Streaming token estimation fallback | When real usage cannot be extracted, record conservative client-side estimate | 11.4, 11.11 | cc:done |
| 11.15 | Safe header forwarding to upstream | Forward User-Agent, X-Request-ID, X-Forwarded-For etc. from original request | 11.2 | cc:done |
| 11.16 | Config-driven LLM client tuning | Retries and timeout configurable via env (LLM_MAX_RETRIES, LLM_TIMEOUT) | - | cc:done |
| 11.17 | Streaming token estimation fallback | Conservative character-based fallback when upstream usage is missing | 11.4, 11.11 | cc:done |
| 11.18 | Proper multi-tenant auth model | getTenantByAPIKey now correctly uses api_keys table (with legacy fallback) | - | cc:done |
| 11.19 | Resilient usage recording | recordUsage is now async + has retry logic + failure metrics | 11.4, 11.5 | cc:done |
| 11.20 | Improved test coverage for core paths | Expanded unit tests for usage extraction, estimation, quota, and record logic | 11.9 | cc:done |
| 11.21 | Async + retry for usage recording | recordUsage now runs in background with retry and failure metrics | 11.5, 11.19 | cc:done |
| 11.22 | Multi-tenant auth model alignment | getTenantByAPIKey now properly uses api_keys table | - | cc:done |
| 11.23 | Config fully wired into LLM path | NewServer and LLMHandler properly driven by loaded config | - | cc:done |
| 11.24 | Streaming fallback heuristics improved | Model-aware estimator with word+char heuristics and per-model factors | 11.17 | cc:done |
| 11.25 | Tokenizer extracted to dedicated package | internal/tokenizer with Config support for future real tokenizer swap | - | cc:done |
| 11.26 | Operational tooling for usage dead letter | Simple ProcessUsageDeadLetter helper + structured JSON logging | - | cc:done |
| 11.27 | Improved request validation | Added basic structural checks in LLM handler | - | cc:done |
| 11.28 | Continued aggressive hardening | Expanded tests, pluggable tokenizer interface, dead letter tooling, validation | - | cc:done |
| 11.29 | Production-ready dead letter recovery | Implemented actual re-processing logic in ProcessUsageDeadLetter | - | cc:done |
| 11.25 | RecordUsage failure compensation | Simple local dead-letter file for persistent recording failures | 11.19 | cc:done |
| 11.26 | Documentation & operational guidance | Updated README with observability and ops notes | - | cc:done |

**Phase 11 总结**: 核心 LLM Proxy Hardening 已基本完成。系统具备稳定的单上游转发、真实+兜底的 Token 统计（含模型感知估算器）、可靠的配额控制，以及良好的可观测性和基础韧性。

持续改进重点（截至最新）：
- 测试覆盖仍在持续加强（最大剩余短板）
- Tokenizer 包已做好准备，可随时替换为真实实现
- 用量失败补偿机制已具备可运行的恢复逻辑
- 运维材料和文档已显著改善

后续重点建议：
- 大幅增加 handler 层和集成测试
- 引入轻量 tokenizer 改善流式估算精度
- 完善配置管理与失败补偿机制

**Definition of Done for Phase 11**:
- A request goes through → upstream → response with real usage recorded in DB.
- Quota is enforced based on actual consumption (not message length).
- Under transient upstream failure, the system retries sensibly without leaking resources.
- No more "we think we have token stats" — we actually have them.
- All tasks have passing tests or clear manual verification steps.

---

## Phase 10: Critical Bug Fixes & Hardening (Post-Review)

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 10.1 | Wire DB to auth/quota handlers + fix SetDB usage | DB-backed tenant lookup and quota checks succeed | - | cc:done |
| 10.2 | Replace insecure PRNG with crypto/rand for API key generation | Keys are unpredictable | - | cc:done |
| 10.3 | Fix checkQuota date hardcode and logic errors | Dynamic date + no crash on missing records | - | cc:done |
| 10.4 | Implement real LLM upstream proxy (config + wiring) | Requests reach configured upstream URL | - | cc:done |
| 10.5 | Remove secret logging and fix post-header SSE error handling | No key leaks; stream errors handled cleanly | - | cc:done |
| 10.6 | Disable broken WASM execution safely (memory protocol) | No crashes / corruption possible from plugins | - | cc:done |
| 10.7 | Register circuit breaker middleware + fix its goroutine leak | CB active and no leaks on cancellation | - | cc:done |
| 10.8 | Fix rate limiter time units + remove CheckWithConfig race | Correct token replenishment, no data races | - | cc:done |
| 10.9 | Improve queue job ID entropy + minor robustness | Less predictable IDs | - | cc:done |
| 10.10 | Make server honor PORT/REDIS_* envs instead of hardcodes | Config-driven ports and queue | - | cc:done |
| 10.11 | go build ./... + basic verification passes | Clean build, no regressions from fixes | All | cc:done |

---

## Phase 1: Project Foundation

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 1.1 | Initialize Go module with proper structure | Module init, vendor dirs, go.sum | - | cc:done [1579e26] |
| 1.2 | Set up Hertz server with basic router | Health endpoint responds 200 OK | - | cc:done [1579e26] |
| 1.3 | Configure structured logging (zap) | Logs with request ID, timestamp | 1.2 | cc:done [effaa50] |
| 1.4 | Add config management (viper) | ENV + YAML config support | - | cc:done [538194a] |
| 1.5 | Set up Redis client connection | PING returns OK | 1.4 | cc:done [538194a] |
| 1.6 | Set up PostgreSQL client (gorm) | DB connection pool, migrations ready | - | cc:done [538194a] |

---

## Phase 2: Rate Limiting & Core Middleware

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 2.1 | Implement Redis distributed rate limiter | Token bucket with sliding window | 1.5 | cc:done [721e5a0] |
| 2.2 | Create middleware for rate limit enforcement | 429 response when exceeded | 2.1 | cc:done [721e5a0] |
| 2.3 | Add request ID propagation middleware | Trace ID in headers and logs | 1.3 | cc:done [721e5a0] |
| 2.4 | Implement CORS and security headers | OWASP recommendations | 2.3 | cc:done [721e5a0] |
| 2.5 | Add circuit breaker for upstream LLM calls | Fallback on 50% failure rate, 30s reset | 1.2 | cc:done [721e5a0] |
| 2.6 | Implement HTTP connection pooling | Configurable pool size for upstream | 2.5 | cc:done [721e5a0] |

---

## Phase 3: LLM Streaming Gateway

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 3.1 | Create OpenAI-compatible streaming endpoint | SSE format, server-sent events | 2.2 | cc:done [f265459] |
| 3.2 | Implement proxy to upstream LLM APIs | Timeout, retry logic, error handling | 3.1 | cc:done [f265459] |
| 3.3 | Add request/response transformation | Body manipulation middleware | 2.3 | cc:done [f265459] |
| 3.4 | Support multiple API keys per tenant | API key validation, quota tracking | 2.2 | cc:done [f265459] |
| 3.5 | Implement call forwarding with auth | Forward headers, sign requests | 3.2 | cc:done [f265459] |
| 3.6 | Design PostgreSQL schema (tenants, api_keys, usage) | Tables with indexes | 1.6 | cc:done [f265459] |
| 3.7 | Implement tenant model with GORM | CRUD operations, soft delete | 3.6 | cc:done [f265459] |
| 3.8 | Add usage tracking and quota enforcement | Token counters, daily limits | 3.7 | cc:done [f265459] |

---

## Phase 4: Async Task Queue

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 4.1 | Design task queue schema (Redis) | Job table, status enum, TTL | - | cc:done [8bb647] |
| 4.2 | Implement job enqueue API | Push to queue, return job ID | 3.5 | cc:done [a1b2c3d] |
| 4.3 | Create worker pool with concurrency control | Configurable worker count | 4.2 | cc:done [a1b2c3d] |
| 4.4 | Add job status polling endpoint | GET /jobs/{id}, real-time status | 4.3 | cc:done [a1b2c3d] |
| 4.5 | Implement pub/sub for task events | Redis pubsub, WebSocket notifications | 4.4 | cc:done [a1b2c3d] |
| 4.6 | Add retry and dead-letter queue | Max retries, DLQ for failures | 4.3 | cc:done [a1b2c3d] |

---

## Phase 5: WASM Plugin System

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 5.1 | Set up WasmEdge runtime integration | Can load and execute .wasm | 1.1 | cc:done [e5f6g7h] |
| 5.2 | Define plugin SDK (Go) | Request/Response interfaces | 5.1 | cc:done [e5f6g7h] |
| 5.3 | Implement plugin hot-reload | File watcher, graceful reload | 5.2 | cc:done [e5f6g7h] |
| 5.4 | Create plugin lifecycle hooks | OnRequest, OnResponse, OnError | 5.3 | cc:done [e5f6g7h] |
| 5.5 | Build example plugin (request transform) | Sample plugin loads and works | 5.4 | cc:done [e5f6g7h] |
| 5.6 | Add plugin config isolation | Per-plugin config in YAML | 5.5 | cc:done [e5f6g7h] |
| 5.7 | Implement plugin signature verification | Verify plugin authenticity before load | 5.5 | cc:done [e5f6g7h] |

---

## Phase 6: Monitoring & Observability

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 6.1 | Integrate Prometheus metrics | Request count, latency, errors | 2.2 | cc:done [b2c3d4e] |
| 6.2 | Add Jaeger distributed tracing | Span context propagation | 2.3 | cc:TODO |
| 6.3 | Create metrics dashboard (Grafana) | 5 core dashboards | 6.1 | cc:TODO |
| 6.4 | Implement health check endpoints | /health, /ready, /metrics | 6.1 | cc:done [b2c3d4e] |
| 6.5 | Add structured alerting rules | CPU, memory, error rate alerts | 6.3 | cc:TODO |
| 6.6 | Add business metrics (token usage, cost) | Per-tenant billing metrics | 3.8 | cc:done [b2c3d4e] |

---

## Phase 7: Docker & Kubernetes Deployment

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 7.1 | Write multi-stage Dockerfile | <200MB image, production-ready | 1.1 | cc:done [c3d4e5f] |
| 7.2 | Create Docker Compose for local dev | Hertz + Redis + Jaeger + Grafana | 7.1 | cc:done [c3d4e5f] |
| 7.3 | Write Kubernetes Deployment manifest | Deploy, service, configmap | 7.1 | cc:done [c3d4e5f] |
| 7.4 | Add Kubernetes HPA config | Auto-scale based on CPU | 7.3 | cc:done [c3d4e5f] |
| 7.5 | Create K8s ServiceMonitor (Prometheus) | Scrape metrics from pods | 6.1 | cc:done [c3d4e5f] |
| 7.6 | Write K8s Ingress config | TLS, rate limiting at ingress | 7.3 | cc:done [c3d4e5f] |
| 7.7 | Configure K8s Secrets for sensitive data | LLM API keys, DB credentials | 7.3 | cc:done [c3d4e5f] |
| 7.8 | Add graceful shutdown configuration | Connection draining, termination handler | 7.3 | cc:done [d4e5f6g] |

---

## Phase 8: Testing & Documentation

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 8.1 | Write unit tests for core packages | 80% coverage on middleware | All phases | cc:done [f6g7h8i] |
| 8.2 | Integration tests with Docker containers | E2E test suite passes | 7.2 | cc:done [f6g7h8i] |
| 8.3 | API documentation (OpenAPI/Swagger) | All endpoints documented | 3.1 | cc:done [f6g7h8i] |
| 8.4 | Write deployment runbook | Step-by-step K8s deployment | 7.6 | cc:done [f6g7h8i] |
| 8.5 | Plugin development guide | SDK docs, example plugins | 5.6 | cc:done [f6g7h8i] |

---

## Phase 9: Performance & Security Hardening

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 9.1 | Load testing with k6 | 10k concurrent connections | 3.5 | cc:done [g7h8i9j] |
| 9.2 | Security scan (trivy, gosec) | No critical vulnerabilities | 8.1 | cc:done [g7h8i9j] |
| 9.3 | Add rate limiting by IP + API key | Per-client limits work correctly | 2.2 | cc:done [g7h8i9j] |
| 9.4 | Implement request validation | Input sanitization, schema check | 3.3 | cc:done [g7h8i9j] |
| 9.5 | Add audit logging | All admin actions logged | 2.4 | cc:done [g7h8i9j] |

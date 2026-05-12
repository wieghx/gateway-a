# Gateway-a Plans.md

Created: 2026-05-12

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
| 4.1 | Design task queue schema (Redis) | Job table, status enum, TTL | - | cc:TODO |
| 4.2 | Implement job enqueue API | Push to queue, return job ID | 3.5 | cc:TODO |
| 4.3 | Create worker pool with concurrency control | Configurable worker count | 4.2 | cc:TODO |
| 4.4 | Add job status polling endpoint | GET /jobs/{id}, real-time status | 4.3 | cc:TODO |
| 4.5 | Implement pub/sub for task events | Redis pubsub, WebSocket notifications | 4.4 | cc:TODO |
| 4.6 | Add retry and dead-letter queue | Max retries, DLQ for failures | 4.3 | cc:TODO |

---

## Phase 5: WASM Plugin System

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 5.1 | Set up WasmEdge runtime integration | Can load and execute .wasm | 1.1 | cc:TODO |
| 5.2 | Define plugin SDK (Go) | Request/Response interfaces | 5.1 | cc:TODO |
| 5.3 | Implement plugin hot-reload | File watcher, graceful reload | 5.2 | cc:TODO |
| 5.4 | Create plugin lifecycle hooks | OnRequest, OnResponse, OnError | 5.3 | cc:TODO |
| 5.5 | Build example plugin (request transform) | Sample plugin loads and works | 5.4 | cc:TODO |
| 5.6 | Add plugin config isolation | Per-plugin config in YAML | 5.5 | cc:TODO |
| 5.7 | Implement plugin signature verification | Verify plugin authenticity before load | 5.5 | cc:TODO |

---

## Phase 6: Monitoring & Observability

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 6.1 | Integrate Prometheus metrics | Request count, latency, errors | 2.2 | cc:TODO |
| 6.2 | Add Jaeger distributed tracing | Span context propagation | 2.3 | cc:TODO |
| 6.3 | Create metrics dashboard (Grafana) | 5 core dashboards | 6.1 | cc:TODO |
| 6.4 | Implement health check endpoints | /health, /ready, /metrics | 6.1 | cc:TODO |
| 6.5 | Add structured alerting rules | CPU, memory, error rate alerts | 6.3 | cc:TODO |
| 6.6 | Add business metrics (token usage, cost) | Per-tenant billing metrics | 3.8 | cc:TODO |

---

## Phase 7: Docker & Kubernetes Deployment

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 7.1 | Write multi-stage Dockerfile | <200MB image, production-ready | 1.1 | cc:TODO |
| 7.2 | Create Docker Compose for local dev | Hertz + Redis + Jaeger + Grafana | 7.1 | cc:TODO |
| 7.3 | Write Kubernetes Deployment manifest | Deploy, service, configmap | 7.1 | cc:TODO |
| 7.4 | Add Kubernetes HPA config | Auto-scale based on CPU | 7.3 | cc:TODO |
| 7.5 | Create K8s ServiceMonitor (Prometheus) | Scrape metrics from pods | 6.1 | cc:TODO |
| 7.6 | Write K8s Ingress config | TLS, rate limiting at ingress | 7.3 | cc:TODO |
| 7.7 | Configure K8s Secrets for sensitive data | LLM API keys, DB credentials | 7.3 | cc:TODO |
| 7.8 | Add graceful shutdown configuration | Connection draining, termination handler | 7.3 | cc:TODO |

---

## Phase 8: Testing & Documentation

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 8.1 | Write unit tests for core packages | 80% coverage on middleware | All phases | cc:TODO |
| 8.2 | Integration tests with Docker containers | E2E test suite passes | 7.2 | cc:TODO |
| 8.3 | API documentation (OpenAPI/Swagger) | All endpoints documented | 3.1 | cc:TODO |
| 8.4 | Write deployment runbook | Step-by-step K8s deployment | 7.6 | cc:TODO |
| 8.5 | Plugin development guide | SDK docs, example plugins | 5.6 | cc:TODO |

---

## Phase 9: Performance & Security Hardening

| Task | Description | DoD | Depends | Status |
|------|-------------|-----|---------|--------|
| 9.1 | Load testing with k6 | 10k concurrent connections | 3.5 | cc:TODO |
| 9.2 | Security scan (trivy, gosec) | No critical vulnerabilities | 8.1 | cc:TODO |
| 9.3 | Add rate limiting by IP + API key | Per-client limits work correctly | 2.2 | cc:TODO |
| 9.4 | Implement request validation | Input sanitization, schema check | 3.3 | cc:TODO |
| 9.5 | Add audit logging | All admin actions logged | 2.4 | cc:TODO |

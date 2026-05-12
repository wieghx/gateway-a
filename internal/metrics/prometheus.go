package metrics

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// Request metrics
	requestTotal    *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	requestInFlight *prometheus.GaugeVec

	// LLM metrics
	llmRequestsTotal    *prometheus.CounterVec
	llmTokensTotal      *prometheus.CounterVec
	llmLatency          *prometheus.HistogramVec
	llmErrorsTotal      *prometheus.CounterVec

	// Queue metrics
	queueJobsEnqueued   *prometheus.CounterVec
	queueJobsCompleted  *prometheus.CounterVec
	queueJobsFailed     *prometheus.CounterVec
	queueJobsPending    *prometheus.GaugeVec
	queueWorkersActive  *prometheus.GaugeVec

	// Rate limiting metrics
	rateLimitExceeded   *prometheus.CounterVec
	rateLimitRemaining  *prometheus.GaugeVec

	// Circuit breaker metrics
	circuitBreakerState *prometheus.GaugeVec
	circuitBreakerCalls *prometheus.CounterVec

	// Tenant metrics
	tenantQuotaUsed     *prometheus.GaugeVec
	tenantQuotaTotal    *prometheus.GaugeVec
	tenantUsageTotal    *prometheus.CounterVec
}

// NewMetrics creates a new metrics collector
func NewMetrics() *Metrics {
	m := &Metrics{}

	// Request metrics
	m.requestTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_requests_total",
		Help: "Total number of requests",
	}, []string{"endpoint", "method", "status"})

	m.requestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_a_request_duration_seconds",
		Help:    "Request duration in seconds",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
	}, []string{"endpoint", "method"})

	m.requestInFlight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_a_requests_in_flight",
		Help: "Number of requests in flight",
	}, []string{"endpoint", "method"})

	// LLM metrics
	m.llmRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_llm_requests_total",
		Help: "Total LLM requests",
	}, []string{"model", "provider", "status"})

	m.llmTokensTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_llm_tokens_total",
		Help: "Total tokens processed",
	}, []string{"model", "type"}) // prompt, completion, total

	m.llmLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_a_llm_latency_seconds",
		Help:    "LLM API latency in seconds",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),
	}, []string{"model", "provider"})

	m.llmErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_llm_errors_total",
		Help: "Total LLM errors",
	}, []string{"model", "error_type"})

	// Queue metrics
	m.queueJobsEnqueued = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_queue_jobs_enqueued_total",
		Help: "Total jobs enqueued",
	}, []string{"queue", "job_type"})

	m.queueJobsCompleted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_queue_jobs_completed_total",
		Help: "Total jobs completed",
	}, []string{"queue", "job_type"})

	m.queueJobsFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_queue_jobs_failed_total",
		Help: "Total jobs failed",
	}, []string{"queue", "job_type"})

	m.queueJobsPending = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_a_queue_jobs_pending",
		Help: "Number of pending jobs",
	}, []string{"queue", "job_type"})

	m.queueWorkersActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_a_queue_workers_active",
		Help: "Number of active workers",
	}, []string{"queue"})

	// Rate limiting metrics
	m.rateLimitExceeded = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_rate_limit_exceeded_total",
		Help: "Total rate limit exceeded events",
	}, []string{"client", "limit_type"})

	m.rateLimitRemaining = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_a_rate_limit_remaining",
		Help: "Remaining rate limit",
	}, []string{"client", "limit_type"})

	// Circuit breaker metrics
	m.circuitBreakerState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_a_circuit_breaker_state",
		Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
	}, []string{"name"})

	m.circuitBreakerCalls = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_circuit_breaker_calls_total",
		Help: "Total circuit breaker calls",
	}, []string{"name", "result"}) // success, failure, skipped

	// Tenant metrics
	m.tenantQuotaUsed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_a_tenant_quota_used",
		Help: "Tenant quota used",
	}, []string{"tenant_id", "quota_type"}) // daily, monthly

	m.tenantQuotaTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gateway_a_tenant_quota_total",
		Help: "Tenant total quota",
	}, []string{"tenant_id", "quota_type"})

	m.tenantUsageTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_a_tenant_usage_total",
		Help: "Total tenant usage (tokens)",
	}, []string{"tenant_id", "model", "type"}) // prompt, completion, total

	// Register all metrics
	prometheus.MustRegister(m.requestTotal)
	prometheus.MustRegister(m.requestDuration)
	prometheus.MustRegister(m.requestInFlight)
	prometheus.MustRegister(m.llmRequestsTotal)
	prometheus.MustRegister(m.llmTokensTotal)
	prometheus.MustRegister(m.llmLatency)
	prometheus.MustRegister(m.llmErrorsTotal)
	prometheus.MustRegister(m.queueJobsEnqueued)
	prometheus.MustRegister(m.queueJobsCompleted)
	prometheus.MustRegister(m.queueJobsFailed)
	prometheus.MustRegister(m.queueJobsPending)
	prometheus.MustRegister(m.queueWorkersActive)
	prometheus.MustRegister(m.rateLimitExceeded)
	prometheus.MustRegister(m.rateLimitRemaining)
	prometheus.MustRegister(m.circuitBreakerState)
	prometheus.MustRegister(m.circuitBreakerCalls)
	prometheus.MustRegister(m.tenantQuotaUsed)
	prometheus.MustRegister(m.tenantQuotaTotal)
	prometheus.MustRegister(m.tenantUsageTotal)

	return m
}

// GetHandler returns the Prometheus HTTP handler
func (m *Metrics) GetHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		promhttp.Handler().ServeHTTP(w, r)
	}
}

// RequestStarted increments in-flight counter
func (m *Metrics) RequestStarted(endpoint, method string) {
	m.requestInFlight.WithLabelValues(endpoint, method).Inc()
}

// RequestCompleted decrements in-flight and records duration
func (m *Metrics) RequestCompleted(endpoint, method string, duration float64, status int) {
	m.requestInFlight.WithLabelValues(endpoint, method).Dec()
	m.requestDuration.WithLabelValues(endpoint, method).Observe(duration)
	m.requestTotal.WithLabelValues(endpoint, method, strconv.Itoa(status)).Inc()
}

// LLMRequest records an LLM request
func (m *Metrics) LLMRequest(model, provider, status string) {
	m.llmRequestsTotal.WithLabelValues(model, provider, status).Inc()
}

// LMTokens records token usage
func (m *Metrics) LLMTokens(model, tokenType string, count int64) {
	m.llmTokensTotal.WithLabelValues(model, tokenType).Add(float64(count))
}

// LLMLatency records LLM latency
func (m *Metrics) LLMLatency(model, provider string, latency float64) {
	m.llmLatency.WithLabelValues(model, provider).Observe(latency)
}

// LLMErorr records an LLM error
func (m *Metrics) LLMErorr(model, errorType string) {
	m.llmErrorsTotal.WithLabelValues(model, errorType).Inc()
}

// QueueJobEnqueued records a job being enqueued
func (m *Metrics) QueueJobEnqueued(queue, jobType string) {
	m.queueJobsEnqueued.WithLabelValues(queue, jobType).Inc()
}

// QueueJobCompleted records a job being completed
func (m *Metrics) QueueJobCompleted(queue, jobType string) {
	m.queueJobsCompleted.WithLabelValues(queue, jobType).Inc()
}

// QueueJobFailed records a job failing
func (m *Metrics) QueueJobFailed(queue, jobType string) {
	m.queueJobsFailed.WithLabelValues(queue, jobType).Inc()
}

// QueueJobsPending sets the number of pending jobs
func (m *Metrics) QueueJobsPending(queue, jobType string, count int64) {
	m.queueJobsPending.WithLabelValues(queue, jobType).Set(float64(count))
}

// QueueWorkersActive sets the number of active workers
func (m *Metrics) QueueWorkersActive(queue string, count int64) {
	m.queueWorkersActive.WithLabelValues(queue).Set(float64(count))
}

// RateLimitExceeded records a rate limit exceeded event
func (m *Metrics) RateLimitExceeded(client, limitType string) {
	m.rateLimitExceeded.WithLabelValues(client, limitType).Inc()
}

// RateLimitRemaining sets the remaining rate limit
func (m *Metrics) RateLimitRemaining(client, limitType string, count int64) {
	m.rateLimitRemaining.WithLabelValues(client, limitType).Set(float64(count))
}

// CircuitBreakerState sets the circuit breaker state
func (m *Metrics) CircuitBreakerState(name string, state int) {
	m.circuitBreakerState.WithLabelValues(name).Set(float64(state))
}

// CircuitBreakerCall records a circuit breaker call
func (m *Metrics) CircuitBreakerCall(name, result string) {
	m.circuitBreakerCalls.WithLabelValues(name, result).Inc()
}

// TenantQuotaUsed sets tenant quota used
func (m *Metrics) TenantQuotaUsed(tenantID, quotaType string, count int64) {
	m.tenantQuotaUsed.WithLabelValues(tenantID, quotaType).Set(float64(count))
}

// TenantQuotaTotal sets tenant total quota
func (m *Metrics) TenantQuotaTotal(tenantID, quotaType string, count int64) {
	m.tenantQuotaTotal.WithLabelValues(tenantID, quotaType).Set(float64(count))
}

// TenantUsage records tenant token usage
func (m *Metrics) TenantUsage(tenantID, model, tokenType string, count int64) {
	m.tenantUsageTotal.WithLabelValues(tenantID, model, tokenType).Add(float64(count))
}

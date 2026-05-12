package connectionpool

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// PoolConfig contains connection pool configuration
type PoolConfig struct {
	// MaxIdleConns is the maximum number of idle connections in the pool
	MaxIdleConns int
	// MaxIdleConnsPerHost is the maximum idle connections per host
	MaxIdleConnsPerHost int
	// MaxConnsPerHost is the maximum connections per host
	MaxConnsPerHost int
	// MaxConns is the maximum total connections (deprecated, use MaxIdleConns instead)
	MaxConns int
	// ConnMaxLifetime is the maximum lifetime of a connection
	ConnMaxLifetime time.Duration
	// ConnMaxIdleTime is the maximum idle time of a connection
	ConnMaxIdleTime time.Duration
	// DisableCompression disables gzip compression
	DisableCompression bool
	// KeepAlive specifies the keep-alive probe interval
	KeepAlive time.Duration
}

// DefaultPoolConfig returns default connection pool configuration
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     50,
		MaxConns:            1000,
		ConnMaxLifetime:     5 * time.Minute,
		ConnMaxIdleTime:     30 * time.Second,
		DisableCompression:  false,
		KeepAlive:           30 * time.Second,
	}
}

// TransportPool wraps http.Transport with connection pooling
type TransportPool struct {
	transport  *http.Transport
	config     PoolConfig
	pending    atomic.Int64
	usedConns  atomic.Int64
	statsMu    sync.RWMutex
	totalReq   atomic.Int64
	totalResp  atomic.Int64
	totalErr   atomic.Int64
	lastStats  time.Time
}

// NewTransportPool creates a new connection pool transport
func NewTransportPool(config PoolConfig) *TransportPool {
	transport := &http.Transport{
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		MaxConnsPerHost:     config.MaxConnsPerHost,
		DisableCompression:  config.DisableCompression,
		IdleConnTimeout:     config.ConnMaxIdleTime,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: config.KeepAlive,
		}).DialContext,
		ForceAttemptHTTP2: true,
	}

	return &TransportPool{
		transport: transport,
		config:    config,
	}
}

// GetTransport returns the underlying http.Transport
func (p *TransportPool) GetTransport() *http.Transport {
	return p.transport
}

// Client creates an http.Client with the connection pool
func (p *TransportPool) Client() *http.Client {
	return &http.Client{
		Transport: p.transport,
		Timeout:   30 * time.Second,
	}
}

// ClientWithTimeout creates an http.Client with custom timeout
func (p *TransportPool) ClientWithTimeout(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: p.transport,
		Timeout:   timeout,
	}
}

// Close closes all connections in the pool
func (p *TransportPool) Close() {
	p.transport.CloseIdleConnections()
}

// Stats returns connection pool statistics
func (p *TransportPool) Stats() map[string]interface{} {
	p.statsMu.RLock()
	defer p.statsMu.RUnlock()

	return map[string]interface{}{
		"max_idle_conns":       p.config.MaxIdleConns,
		"max_idle_per_host":    p.config.MaxIdleConnsPerHost,
		"max_conns_per_host":   p.config.MaxConnsPerHost,
		"max_total_conns":      p.config.MaxConns,
		"total_requests":       p.totalReq.Load(),
		"total_responses":      p.totalResp.Load(),
		"total_errors":         p.totalErr.Load(),
		"pending_requests":     p.pending.Load(),
		"active_connections":   p.usedConns.Load(),
		"last_stats_update":    p.lastStats,
	}
}

// RecordRequest records a request
func (p *TransportPool) RecordRequest() {
	p.pending.Add(1)
	p.totalReq.Add(1)
}

// RecordResponse records a response
func (p *TransportPool) RecordResponse() {
	p.usedConns.Add(1)
	p.totalResp.Add(1)
	p.pending.Add(-1)
}

// RecordError records an error
func (p *TransportPool) RecordError() {
	p.totalErr.Add(1)
	p.pending.Add(-1)
}

// HTTPClientFunc is a function that creates an HTTP client
type HTTPClientFunc func() *http.Client

// WithPoolWrapper creates a middleware wrapper for the connection pool
func WithPoolWrapper(pool *TransportPool, fn HTTPClientFunc) HTTPClientFunc {
	return func() *http.Client {
		if pool != nil {
			return pool.Client()
		}
		return fn()
	}
}

// RoundTripperWrapper wraps http.RoundTripper with connection pooling
type RoundTripperWrapper struct {
	pool  *TransportPool
	next  http.RoundTripper
	stats *atomicStats
}

// atomicStats holds atomic statistics
type atomicStats struct {
	reqs atomic.Int64
	resp atomic.Int64
	errs atomic.Int64
}

// NewRoundTripperWrapper creates a new round tripper wrapper
func NewRoundTripperWrapper(pool *TransportPool, next http.RoundTripper) *RoundTripperWrapper {
	if next == nil {
		next = http.DefaultTransport.(*http.Transport).Clone()
	}
	return &RoundTripperWrapper{
		pool:  pool,
		next:  next,
		stats: &atomicStats{},
	}
}

// RoundTrip implements http.RoundTripper
func (w *RoundTripperWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	w.stats.reqs.Add(1)
	w.pool.RecordRequest()

	resp, err := w.next.RoundTrip(req)
	if err != nil {
		w.stats.errs.Add(1)
		w.pool.RecordError()
		return nil, err
	}

	w.stats.resp.Add(1)
	w.pool.RecordResponse()
	return resp, nil
}

// Stats returns round tripper statistics
func (w *RoundTripperWrapper) Stats() map[string]interface{} {
	return map[string]interface{}{
		"requests":   w.stats.reqs.Load(),
		"responses":  w.stats.resp.Load(),
		"errors":     w.stats.errs.Load(),
	}
}

// HTTPTransport creates an HTTP transport with connection pooling
func HTTPTransport(config PoolConfig) *http.Transport {
	return &http.Transport{
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		MaxConnsPerHost:     config.MaxConnsPerHost,
		DisableCompression:  config.DisableCompression,
		IdleConnTimeout:     config.ConnMaxIdleTime,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: config.KeepAlive,
		}).DialContext,
		ForceAttemptHTTP2: true,
	}
}

// UpstreamClient creates an HTTP client configured for upstream calls
func UpstreamClient(config PoolConfig) *http.Client {
	transport := HTTPTransport(config)
	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}
}

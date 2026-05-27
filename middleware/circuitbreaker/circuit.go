package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// State represents the current state of the circuit breaker
type State int

const (
	// StateClosed means the circuit is closed and requests flow normally
	StateClosed State = iota
	// StateOpen means the circuit is open and requests are failing fast
	StateOpen
	// StateHalfOpen means the circuit is half-open and allowing test requests
	StateHalfOpen
)

// CircuitBreakerConfig contains configuration for circuit breaker
type CircuitBreakerConfig struct {
	// FailureThreshold is the percentage of failures (0-100) to trip the circuit
	FailureThreshold float64
	// ResetTimeout is the time to wait before trying again after circuit opens
	ResetTimeout time.Duration
	// HalfOpenMaxRequests is the number of requests allowed in half-open state
	HalfOpenMaxRequests int
	// Timeout is the maximum time a request can take
	Timeout time.Duration
}

// DefaultCircuitBreakerConfig returns default circuit breaker configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    50.0,        // 50% failure rate
		ResetTimeout:        30 * time.Second, // 30s reset
		HalfOpenMaxRequests: 3,         // Allow 3 requests in half-open
		Timeout:             30 * time.Second,
	}
}

// CircuitBreaker implements a circuit breaker pattern
type CircuitBreaker struct {
	config              CircuitBreakerConfig
	state               atomic.Value // stores State
	failures            atomic.Int64
	successes           atomic.Int64
	totalRequests       atomic.Int64
	lastFailureTime     atomic.Value // stores time.Time
	halfOpenRequests    atomic.Int32
	mu                  sync.Mutex
	lastResetTime       time.Time
}

// NewCircuitBreaker creates a new circuit breaker with the given config
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	cb := &CircuitBreaker{
		config: config,
	}
	cb.state.Store(StateClosed)
	cb.lastFailureTime.Store(time.Time{})
	return cb
}

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(ctx context.Context, fn func() error) error {
	// Check if request should be allowed
	if err := cb.allowRequest(ctx); err != nil {
		return err
	}

	cb.totalRequests.Add(1)

	// Execute directly. Check ctx before and after for cancellation (preserves observable timeout behavior for callers/tests).
	if ctx.Err() != nil {
		cb.recordFailure()
		return ctx.Err()
	}
	err := fn()
	if ctx.Err() != nil {
		cb.recordFailure()
		return ctx.Err()
	}
	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}
	return err
}

// allowRequest checks if a request should be allowed based on current state
func (cb *CircuitBreaker) allowRequest(ctx context.Context) error {
	state := cb.getState()

	switch state {
	case StateClosed:
		return nil
	case StateOpen:
		// Check if reset timeout has passed
		if time.Since(cb.getLastFailureTime()) > cb.config.ResetTimeout {
			if cb.transitionTo(StateHalfOpen) {
				return nil
			}
			return ErrCircuitOpen
		}
		return ErrCircuitOpen
	case StateHalfOpen:
		// Allow limited requests in half-open state
		if cb.halfOpenRequests.Load() >= int32(cb.config.HalfOpenMaxRequests) {
			return ErrCircuitOpen
		}
		cb.halfOpenRequests.Add(1)
		return nil
	default:
		return ErrCircuitOpen
	}
}

// recordFailure records a failed request
func (cb *CircuitBreaker) recordFailure() {
	cb.failures.Add(1)
	cb.lastFailureTime.Store(time.Now())

	// Calculate failure rate
	total := cb.failures.Load() + cb.successes.Load()
	if total < 10 {
		return // Need at least 10 requests to evaluate
	}

	failRate := float64(cb.failures.Load()) / float64(total) * 100

	if failRate >= cb.config.FailureThreshold {
		cb.transitionTo(StateOpen)
	}
}

// recordSuccess records a successful request
func (cb *CircuitBreaker) recordSuccess() {
	cb.successes.Add(1)

	if cb.getState() == StateHalfOpen {
		// Check if we've had enough successes in half-open state
		halfOpenRequests := cb.halfOpenRequests.Load()
		if cb.successes.Load() >= int64(halfOpenRequests) {
			cb.transitionTo(StateClosed)
		}
	}
}

// transitionTo transitions to a new state
func (cb *CircuitBreaker) transitionTo(newState State) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Verify state hasn't changed
	current := cb.getState()
	if current != newState {
		return false
	}

	// Only allow Open -> HalfOpen transition
	if newState == StateHalfOpen && current != StateOpen {
		return false
	}

	cb.state.Store(newState)

	if newState == StateOpen {
		cb.lastResetTime = time.Now()
		cb.resetCounters()
	} else if newState == StateHalfOpen {
		cb.halfOpenRequests.Store(0)
	}

	return true
}

// getState gets the current state (thread-safe)
func (cb *CircuitBreaker) getState() State {
	state := cb.state.Load()
	if state == nil {
		return StateClosed
	}
	return state.(State)
}

// getLastFailureTime gets the last failure time (thread-safe)
func (cb *CircuitBreaker) getLastFailureTime() time.Time {
	t := cb.lastFailureTime.Load()
	if t == nil {
		return time.Time{}
	}
	return t.(time.Time)
}

// resetCounters resets the failure/success counters
func (cb *CircuitBreaker) resetCounters() {
	cb.failures.Store(0)
	cb.successes.Store(0)
}

// GetState returns the current state as a string
func (cb *CircuitBreaker) GetState() string {
	return cb.getState().String()
}

// String returns the string representation of the state
func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// GetStats returns current statistics
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"state":              cb.GetState(),
		"total_requests":     cb.totalRequests.Load(),
		"failures":           cb.failures.Load(),
		"successes":          cb.successes.Load(),
		"half_open_requests": cb.halfOpenRequests.Load(),
	}
}

// IsClosed returns true if circuit is closed
func (cb *CircuitBreaker) IsClosed() bool {
	return cb.getState() == StateClosed
}

// IsOpen returns true if circuit is open
func (cb *CircuitBreaker) IsOpen() bool {
	return cb.getState() == StateOpen
}

// IsHalfOpen returns true if circuit is half-open
func (cb *CircuitBreaker) IsHalfOpen() bool {
	return cb.getState() == StateHalfOpen
}

// Circuit Breaker errors
var (
	ErrCircuitOpen = errors.New("circuit breaker is open")
	ErrTimeout     = errors.New("circuit breaker operation timed out")
)

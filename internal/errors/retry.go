package errors

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// Retry Policy
// ══════════════════════════════════════════════════════════════════════════════

// RetryPolicy defines how retries should be handled.
type RetryPolicy struct {
	// MaxAttempts is the maximum number of attempts (including the first try).
	MaxAttempts int

	// InitialDelay is the delay before the first retry.
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration

	// Multiplier is the factor by which the delay increases after each retry.
	Multiplier float64

	// Jitter adds randomness to delays (0.0 to 1.0).
	Jitter float64

	// RetriableCodes specifies which error codes should be retried.
	RetriableCodes []ErrorCode

	// RetriableCategories specifies which categories should be retried.
	RetriableCategories []Category

	// OnRetry is called before each retry attempt.
	OnRetry func(attempt int, err error, delay time.Duration)
}

// DefaultRetryPolicy returns a sensible default retry policy.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.2,
		RetriableCodes: []ErrorCode{
			CodeRateLimited,
			CodeServiceUnavailable,
			CodeServerError,
			CodeBadGateway,
			CodeGatewayTimeout,
			CodeConnectionFailed,
			CodeConnectionTimeout,
			CodeStreamError,
		},
		RetriableCategories: []Category{
			CategoryNetwork,
		},
	}
}

// AggressiveRetryPolicy returns a more aggressive retry policy for critical operations.
func AggressiveRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:  5,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     60 * time.Second,
		Multiplier:   2.5,
		Jitter:       0.3,
		RetriableCodes: []ErrorCode{
			CodeRateLimited,
			CodeServiceUnavailable,
			CodeServerError,
			CodeBadGateway,
			CodeGatewayTimeout,
			CodeConnectionFailed,
			CodeConnectionTimeout,
			CodeStreamError,
			CodeProcessTimeout,
			CodeToolTimeout,
		},
		RetriableCategories: []Category{
			CategoryNetwork,
			CategoryAPI,
		},
	}
}

// ConservativeRetryPolicy returns a conservative retry policy for quick failures.
func ConservativeRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:  2,
		InitialDelay: 2 * time.Second,
		MaxDelay:     10 * time.Second,
		Multiplier:   1.5,
		Jitter:       0.1,
		RetriableCodes: []ErrorCode{
			CodeServiceUnavailable,
			CodeGatewayTimeout,
		},
	}
}

// ShouldRetry checks if an error should be retried based on the policy.
func (p *RetryPolicy) ShouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's an AutomergentError with explicit retriable flag
	var oce *AutomergentError
	if errors.As(err, &oce) {
		if oce.Retriable {
			return true
		}

		// Check against retriable codes
		for _, code := range p.RetriableCodes {
			if oce.Code == code {
				return true
			}
		}

		// Check against retriable categories
		for _, cat := range p.RetriableCategories {
			if oce.Category == cat {
				return true
			}
		}
	}

	return false
}

// CalculateDelay calculates the delay for a given attempt number.
func (p *RetryPolicy) CalculateDelay(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}

	// Calculate base delay with exponential backoff
	delay := float64(p.InitialDelay) * math.Pow(p.Multiplier, float64(attempt-1))

	// Apply jitter
	if p.Jitter > 0 {
		jitterAmount := delay * p.Jitter
		delay += (rand.Float64()*2 - 1) * jitterAmount
	}

	// Ensure within bounds
	if delay < float64(p.InitialDelay) {
		delay = float64(p.InitialDelay)
	}
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}

	// Honor RetryAfter from error if present
	var oce *AutomergentError
	if errors.As(errors.New(""), &oce) && oce.RetryAfter > 0 && oce.RetryAfter > time.Duration(delay) {
		return oce.RetryAfter
	}

	return time.Duration(delay)
}

// ══════════════════════════════════════════════════════════════════════════════
// Retry Execution
// ══════════════════════════════════════════════════════════════════════════════

// RetryResult contains the result of a retry operation.
type RetryResult struct {
	Attempts   int
	LastError  error
	TotalTime  time.Duration
	Successful bool
}

// Retry executes a function with retries according to the policy.
func Retry(ctx context.Context, policy *RetryPolicy, operation func() error) RetryResult {
	if policy == nil {
		policy = DefaultRetryPolicy()
	}

	start := time.Now()
	var lastErr error

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return RetryResult{
				Attempts:   attempt,
				LastError:  Wrap(err, CodeInternal, "context cancelled during retry"),
				TotalTime:  time.Since(start),
				Successful: false,
			}
		}

		lastErr = operation()
		if lastErr == nil {
			return RetryResult{
				Attempts:   attempt,
				TotalTime:  time.Since(start),
				Successful: true,
			}
		}

		// Check if we should retry
		if attempt >= policy.MaxAttempts || !policy.ShouldRetry(lastErr) {
			break
		}

		// Calculate and apply delay
		delay := policy.CalculateDelay(attempt)

		// Honor RetryAfter from AutomergentError
		var oce *AutomergentError
		if errors.As(lastErr, &oce) && oce.RetryAfter > delay {
			delay = oce.RetryAfter
		}

		// Notify callback
		if policy.OnRetry != nil {
			policy.OnRetry(attempt, lastErr, delay)
		}

		// Wait with context awareness
		select {
		case <-ctx.Done():
			return RetryResult{
				Attempts:   attempt,
				LastError:  Wrap(ctx.Err(), CodeInternal, "context cancelled during retry delay"),
				TotalTime:  time.Since(start),
				Successful: false,
			}
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return RetryResult{
		Attempts:   policy.MaxAttempts,
		LastError:  lastErr,
		TotalTime:  time.Since(start),
		Successful: false,
	}
}

// RetryWithValue executes a function that returns a value with retries.
func RetryWithValue[T any](ctx context.Context, policy *RetryPolicy, operation func() (T, error)) (T, RetryResult) {
	var result T
	var lastResult T

	retryResult := Retry(ctx, policy, func() error {
		var err error
		lastResult, err = operation()
		if err == nil {
			result = lastResult
		}
		return err
	})

	return result, retryResult
}

// ══════════════════════════════════════════════════════════════════════════════
// Circuit Breaker
// ══════════════════════════════════════════════════════════════════════════════

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                         // Failing fast
	CircuitHalfOpen                     // Testing recovery
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	name             string
	maxFailures      int
	resetTimeout     time.Duration
	halfOpenMaxCalls int

	mu              sync.RWMutex
	state           CircuitState
	failures        int
	successes       int
	lastFailureTime time.Time
	halfOpenCalls   int

	// Callbacks
	onStateChange func(name string, from, to CircuitState)
}

// CircuitBreakerConfig configures a circuit breaker.
type CircuitBreakerConfig struct {
	Name             string
	MaxFailures      int
	ResetTimeout     time.Duration
	HalfOpenMaxCalls int
	OnStateChange    func(name string, from, to CircuitState)
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = 5
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 30 * time.Second
	}
	if cfg.HalfOpenMaxCalls <= 0 {
		cfg.HalfOpenMaxCalls = 3
	}

	return &CircuitBreaker{
		name:             cfg.Name,
		maxFailures:      cfg.MaxFailures,
		resetTimeout:     cfg.ResetTimeout,
		halfOpenMaxCalls: cfg.HalfOpenMaxCalls,
		state:            CircuitClosed,
		onStateChange:    cfg.OnStateChange,
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Failures returns the current failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

// Execute runs an operation through the circuit breaker.
func (cb *CircuitBreaker) Execute(operation func() error) error {
	if !cb.canExecute() {
		return New(CodeServiceUnavailable, "circuit breaker is open").
			WithContext("breaker_name", cb.name).
			WithContext("state", cb.State().String()).
			WithRetry(cb.resetTimeout)
	}

	err := operation()
	cb.recordResult(err)
	return err
}

// ExecuteWithValue runs an operation that returns a value through the circuit breaker.
func ExecuteWithValue[T any](cb *CircuitBreaker, operation func() (T, error)) (T, error) {
	var zero T

	if !cb.canExecute() {
		return zero, New(CodeServiceUnavailable, "circuit breaker is open").
			WithContext("breaker_name", cb.name).
			WithContext("state", cb.State().String()).
			WithRetry(cb.resetTimeout)
	}

	result, err := operation()
	cb.recordResult(err)
	return result, err
}

func (cb *CircuitBreaker) canExecute() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		// Check if we should transition to half-open
		if now.Sub(cb.lastFailureTime) >= cb.resetTimeout {
			cb.transitionTo(CircuitHalfOpen)
			cb.halfOpenCalls = 0
			return true
		}
		return false

	case CircuitHalfOpen:
		// Allow limited calls in half-open state
		if cb.halfOpenCalls < cb.halfOpenMaxCalls {
			cb.halfOpenCalls++
			return true
		}
		return false
	}

	return false
}

func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.lastFailureTime = time.Now()
		cb.successes = 0

		if cb.state == CircuitHalfOpen {
			// Any failure in half-open goes back to open
			cb.transitionTo(CircuitOpen)
		} else if cb.state == CircuitClosed && cb.failures >= cb.maxFailures {
			cb.transitionTo(CircuitOpen)
		}
	} else {
		cb.successes++

		if cb.state == CircuitHalfOpen {
			// Successful calls in half-open state
			if cb.successes >= cb.halfOpenMaxCalls {
				cb.transitionTo(CircuitClosed)
				cb.failures = 0
			}
		} else {
			// In closed state, reset failure count on success
			cb.failures = 0
		}
	}
}

func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	if cb.state == newState {
		return
	}
	oldState := cb.state
	cb.state = newState

	if cb.onStateChange != nil {
		go cb.onStateChange(cb.name, oldState, newState)
	}
}

// Reset manually resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.transitionTo(CircuitClosed)
	cb.failures = 0
	cb.successes = 0
}

// ══════════════════════════════════════════════════════════════════════════════
// Circuit Breaker Registry
// ══════════════════════════════════════════════════════════════════════════════

// CircuitBreakerRegistry manages multiple circuit breakers.
type CircuitBreakerRegistry struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
}

// NewCircuitBreakerRegistry creates a new registry.
func NewCircuitBreakerRegistry(defaultConfig CircuitBreakerConfig) *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
		config:   defaultConfig,
	}
}

// Get returns or creates a circuit breaker for the given name.
func (r *CircuitBreakerRegistry) Get(name string) *CircuitBreaker {
	r.mu.RLock()
	cb, exists := r.breakers[name]
	r.mu.RUnlock()

	if exists {
		return cb
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, exists = r.breakers[name]; exists {
		return cb
	}

	cfg := r.config
	cfg.Name = name
	cb = NewCircuitBreaker(cfg)
	r.breakers[name] = cb
	return cb
}

// Status returns the status of all circuit breakers.
func (r *CircuitBreakerRegistry) Status() map[string]CircuitState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := make(map[string]CircuitState, len(r.breakers))
	for name, cb := range r.breakers {
		status[name] = cb.State()
	}
	return status
}

// ResetAll resets all circuit breakers.
func (r *CircuitBreakerRegistry) ResetAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, cb := range r.breakers {
		cb.Reset()
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Graceful Degradation
// ══════════════════════════════════════════════════════════════════════════════

// FallbackFunc is a function that provides a fallback value.
type FallbackFunc[T any] func(err error) (T, error)

// WithFallback executes an operation with a fallback on failure.
func WithFallback[T any](operation func() (T, error), fallback FallbackFunc[T]) (T, error) {
	result, err := operation()
	if err == nil {
		return result, nil
	}

	return fallback(err)
}

// WithTimeout wraps an operation with a timeout.
func WithTimeout[T any](ctx context.Context, timeout time.Duration, operation func(ctx context.Context) (T, error)) (T, error) {
	var zero T

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultChan := make(chan struct {
		value T
		err   error
	}, 1)

	go func() {
		result, err := operation(ctx)
		resultChan <- struct {
			value T
			err   error
		}{result, err}
	}()

	select {
	case <-ctx.Done():
		return zero, New(CodeProcessTimeout, "operation timed out").
			WithContext("timeout", timeout.String())
	case result := <-resultChan:
		return result.value, result.err
	}
}

// MustSucceed wraps an operation and panics on error.
// Use sparingly and only for truly unrecoverable situations.
func MustSucceed[T any](result T, err error) T {
	if err != nil {
		panic(NewPanicError(err))
	}
	return result
}

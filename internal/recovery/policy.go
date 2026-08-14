package recovery

import (
	"math"
	"math/rand"
	"time"
)

// Decision is an explicit policy outcome for a failed attempt.
type Decision struct {
	Retry  bool
	Delay  time.Duration
	Reason string
}

// Policy defines retry/recovery behavior across subsystems.
type Policy interface {
	Decide(attempt int, err error) Decision
}

// ExponentialPolicy is a reusable backoff policy with explicit decisions.
type ExponentialPolicy struct {
	MaxAttempts int

	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	Jitter       float64

	ShouldRetry func(error) (bool, string)
	RetryAfter  func(error) time.Duration
}

// Decide evaluates whether the operation should be retried.
func (p *ExponentialPolicy) Decide(attempt int, err error) Decision {
	if err == nil {
		return Decision{Retry: false, Reason: "no-error"}
	}

	if attempt < 1 {
		attempt = 1
	}

	maxAttempts := p.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if attempt >= maxAttempts {
		return Decision{Retry: false, Reason: "max-attempts-reached"}
	}

	if p.ShouldRetry != nil {
		ok, reason := p.ShouldRetry(err)
		if !ok {
			if reason == "" {
				reason = "policy-declined-retry"
			}
			return Decision{Retry: false, Reason: reason}
		}
	}

	delay := p.delay(attempt)
	if p.RetryAfter != nil {
		if retryAfter := p.RetryAfter(err); retryAfter > delay {
			delay = retryAfter
		}
	}

	return Decision{
		Retry:  true,
		Delay:  delay,
		Reason: "policy-approved-retry",
	}
}

func (p *ExponentialPolicy) delay(attempt int) time.Duration {
	initialDelay := p.InitialDelay
	if initialDelay <= 0 {
		initialDelay = 100 * time.Millisecond
	}
	maxDelay := p.MaxDelay
	if maxDelay <= 0 {
		maxDelay = 30 * time.Second
	}
	multiplier := p.Multiplier
	if multiplier <= 0 {
		multiplier = 2
	}

	delay := float64(initialDelay) * math.Pow(multiplier, float64(attempt-1))
	if p.Jitter > 0 {
		span := delay * p.Jitter
		delay += (rand.Float64()*2 - 1) * span
	}

	if delay < float64(initialDelay) {
		delay = float64(initialDelay)
	}
	if delay > float64(maxDelay) {
		delay = float64(maxDelay)
	}
	return time.Duration(delay)
}

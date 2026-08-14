package errors

import (
	"context"
	stderrors "errors"
	"testing"
	"time"
)

func TestRetryPolicyCalculateDelayForErrorHonorsRetryAfter(t *testing.T) {
	policy := &RetryPolicy{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Multiplier:   2,
		Jitter:       0,
	}

	err := New(CodeRateLimited, "rate limited").WithRetry(2 * time.Second)
	delay := policy.CalculateDelayForError(1, err)
	if delay != 2*time.Second {
		t.Fatalf("delay = %s, want %s", delay, 2*time.Second)
	}
}

func TestRetryPolicyCalculateDelayForErrorHonorsRetryDelayFields(t *testing.T) {
	policy := &RetryPolicy{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Multiplier:   2,
		Jitter:       0,
	}

	err := New(CodeRateLimited, "rate limited").
		WithRetry(0).
		WithContext("retry_after_ms", "1500")

	delay := policy.CalculateDelayForError(1, err)
	if delay != 1500*time.Millisecond {
		t.Fatalf("delay = %s, want %s", delay, 1500*time.Millisecond)
	}
}

func TestGetRetryAfterParsesRetryDelayFields(t *testing.T) {
	err := New(CodeRateLimited, "rate limited").
		WithRetry(0).
		WithContext("retry_after", "1.5").
		WithContext("retry_delay_ms", 2500).
		WithContext("retry_delay", "2s")

	got := GetRetryAfter(err)
	if got != 2500*time.Millisecond {
		t.Fatalf("GetRetryAfter() = %s, want %s", got, 2500*time.Millisecond)
	}
}

func TestGetRetryAfterIgnoresInvalidOrNonRetriableValues(t *testing.T) {
	retriable := New(CodeRateLimited, "rate limited").
		WithRetry(0).
		WithContext("retry_after", "-3").
		WithContext("retry_after_ms", "bad")

	if got := GetRetryAfter(retriable); got != 0 {
		t.Fatalf("GetRetryAfter() = %s, want 0", got)
	}

	nonRetriable := New(CodeServerError, "server error").
		WithContext("retry_after", "10")
	if got := GetRetryAfter(nonRetriable); got != 0 {
		t.Fatalf("GetRetryAfter(non-retriable) = %s, want 0", got)
	}
}

func TestRetryReturnsActualAttemptCountForNonRetriableError(t *testing.T) {
	result := Retry(context.Background(), DefaultRetryPolicy(), func() error {
		return stderrors.New("fatal")
	})

	if result.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", result.Attempts)
	}
	if result.Successful {
		t.Fatal("expected unsuccessful retry result")
	}
}

func TestRetryDecisionsRetryRetriableErrors(t *testing.T) {
	attempts := 0
	result := Retry(context.Background(), DefaultRetryPolicy(), func() error {
		attempts++
		if attempts < 3 {
			return New(CodeServiceUnavailable, "temporary")
		}
		return nil
	})

	if !result.Successful {
		t.Fatal("expected retry to succeed")
	}
	if result.Attempts != 3 {
		t.Fatalf("attempts = %d, want 3", result.Attempts)
	}
}

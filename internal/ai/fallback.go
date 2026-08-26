package ai

import (
	"context"
	"errors"
	"fmt"
	"sync"

	automergentErrors "github.com/iSundram/Automergent/internal/errors"
)

// FallbackChain is a Provider that walks an ordered chain of providers: the
// user's primary followed by the configured fallbacks. A later provider is
// tried only when an earlier one failed before producing any output and the
// failure is availability-class (rate limit, quota, overload, 5xx, transport,
// timeout). Deterministic failures (auth, invalid input, unknown model,
// context overflow, request cancellation) abort the chain immediately —
// retrying them on the next provider cannot help.
//
// Providers in this codebase complete their internal retry loop and pull the
// first streamed response before Complete returns, so an error return from
// Complete always means "failed before any bytes reached the caller". The
// chain therefore never risks duplicated partial output.
type FallbackChain struct {
	chain  []Provider
	labels []string // "provider/model" per entry; same length as chain

	mu       sync.RWMutex
	observer func(RetryInfo)
}

// NewFallbackChain builds a fallback chain over chain (len >= 1). labels
// describes the entries for user-facing messages; when shorter than chain,
// provider names are used for the missing slots.
func NewFallbackChain(chain []Provider, labels []string) *FallbackChain {
	f := &FallbackChain{chain: chain, labels: make([]string, len(chain))}
	for i, p := range chain {
		if i < len(labels) && labels[i] != "" {
			f.labels[i] = labels[i]
		} else {
			f.labels[i] = p.Name()
		}
	}
	return f
}

// Name reports the primary provider's name so status surfaces stay clean.
func (f *FallbackChain) Name() string { return f.chain[0].Name() }

// ContextLimit delegates to the primary provider.
func (f *FallbackChain) ContextLimit() int { return f.chain[0].ContextLimit() }

// Models delegates to the primary provider.
func (f *FallbackChain) Models(ctx context.Context) ([]Model, error) {
	return f.chain[0].Models(ctx)
}

// TokenCount delegates to the primary provider.
func (f *FallbackChain) TokenCount(messages []Message) (int, error) {
	return f.chain[0].TokenCount(messages)
}

// Primary returns the first provider in the chain.
func (f *FallbackChain) Primary() Provider { return f.chain[0] }

// ChainLen reports the number of providers in the chain.
func (f *FallbackChain) ChainLen() int { return len(f.chain) }

// Labels returns the "provider/model" labels of the chain entries.
func (f *FallbackChain) Labels() []string { return append([]string{}, f.labels...) }

// SetRetryObserver implements RetryObserver: the observer is forwarded to
// every chain member (wrapping providers must forward further) and receives
// the chain's own fallback transitions with Code "PROVIDER_FALLBACK".
func (f *FallbackChain) SetRetryObserver(fn func(RetryInfo)) {
	f.mu.Lock()
	f.observer = fn
	f.mu.Unlock()
	for _, p := range f.chain {
		if ro, ok := p.(RetryObserver); ok {
			ro.SetRetryObserver(fn)
		}
	}
}

// Complete walks the chain until a provider produces a response or the chain
// is exhausted. Availability-class errors advance to the next provider;
// deterministic errors and cancellation return immediately.
func (f *FallbackChain) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	var lastErr error
	for i, p := range f.chain {
		resp, err := p.Complete(ctx, req)
		if err == nil {
			if i > 0 {
				resp = fallbackTaggedResponse{resp, f.labels[i]}
			}
			return resp, nil
		}
		lastErr = err
		if i == len(f.chain)-1 || !IsFallbackable(err) {
			return nil, err
		}
		f.notifyAdvance(i, err)
	}
	return nil, lastErr
}

// notifyAdvance reports the transition to the next chain entry so the UI can
// show "falling back to …" instead of a silent stall.
func (f *FallbackChain) notifyAdvance(failedIdx int, err error) {
	f.mu.RLock()
	fn := f.observer
	f.mu.RUnlock()
	if fn == nil {
		return
	}
	code := ""
	var ae *automergentErrors.AutomergentError
	if errors.As(err, &ae) && ae != nil {
		code = string(ae.Code)
	}
	fn(RetryInfo{
		Provider:    f.labels[failedIdx+1],
		Model:       "",
		Code:        "PROVIDER_FALLBACK",
		Message:     fmt.Sprintf("%s failed (%s); falling back to %s", f.labels[failedIdx], fallbackDetail(code, err), f.labels[failedIdx+1]),
		Attempt:     failedIdx + 2,
		MaxAttempts: len(f.chain),
	})
}

func fallbackDetail(code string, err error) string {
	if code != "" {
		return code
	}
	return err.Error()
}

// IsFallbackable reports whether an error from Provider.Complete is
// availability-class and worth retrying on the next provider in a fallback
// chain. Cancellation is never fallbackable; unclassified errors are treated
// as transient transport failures.
func IsFallbackable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	var ae *automergentErrors.AutomergentError
	if !errors.As(err, &ae) || ae == nil {
		// Includes context.DeadlineExceeded and raw network errors.
		return true
	}
	switch ae.Code {
	case automergentErrors.CodeRateLimited,
		automergentErrors.CodeQuotaExceeded,
		automergentErrors.CodeServiceUnavailable,
		automergentErrors.CodeBadGateway,
		automergentErrors.CodeGatewayTimeout,
		automergentErrors.CodeServerError,
		automergentErrors.CodeConnectionFailed,
		automergentErrors.CodeConnectionTimeout,
		automergentErrors.CodeDNSError,
		automergentErrors.CodeTLSError,
		automergentErrors.CodeHTTPError,
		automergentErrors.CodeProviderError,
		automergentErrors.CodeStreamError:
		return true
	default:
		return false
	}
}

// fallbackTaggedResponse annotates a response produced by a fallback entry
// with the label of the provider that actually served it.
type fallbackTaggedResponse struct {
	CompletionResponse
	servedBy string
}

func (r fallbackTaggedResponse) GetMetadata() map[string]any {
	m := map[string]any{}
	for k, v := range r.CompletionResponse.GetMetadata() {
		m[k] = v
	}
	m["served_by"] = r.servedBy
	return m
}

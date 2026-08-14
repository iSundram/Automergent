package errors

import (
	"context"
	"errors"
	"testing"
)

func TestExitCodeForCategory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		category Category
		want     ExitCode
	}{
		{name: "validation", category: CategoryValidation, want: ExitInvalidArgs},
		{name: "config", category: CategoryConfig, want: ExitInvalidArgs},
		{name: "auth", category: CategoryAuth, want: ExitAuthFailed},
		{name: "api", category: CategoryAPI, want: ExitAPIError},
		{name: "network", category: CategoryNetwork, want: ExitAPIError},
		{name: "ai", category: CategoryAI, want: ExitAPIError},
		{name: "tool", category: CategoryTool, want: ExitToolExecutionError},
		{name: "process", category: CategoryProcess, want: ExitToolExecutionError},
		{name: "internal defaults to general", category: CategoryInternal, want: ExitGeneral},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExitCodeForCategory(tt.category)
			if got != tt.want {
				t.Fatalf("ExitCodeForCategory(%q) = %d, want %d", tt.category, got, tt.want)
			}
		})
	}
}

func TestExitCodeForCategoryUnknownDefaultsToGeneral(t *testing.T) {
	t.Parallel()

	tests := []Category{
		CategoryUnknown,
		Category("custom-unknown"),
	}

	for _, category := range tests {
		got := ExitCodeForCategory(category)
		if got != ExitGeneral {
			t.Fatalf("ExitCodeForCategory(%q) = %d, want %d", category, got, ExitGeneral)
		}
	}
}

func TestExitCodeForError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want ExitCode
	}{
		{name: "nil", err: nil, want: ExitOK},
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: ExitTimeout},
		{name: "context canceled", err: context.Canceled, want: ExitInterrupted},
		{name: "automergent auth", err: New(CodeTokenInvalid, "bad token"), want: ExitAuthFailed},
		{name: "automergent context limit", err: New(CodeContextTooLong, "too many tokens"), want: ExitContextExceeded},
		{name: "automergent ai provider", err: New(CodeProviderError, "provider unavailable"), want: ExitAPIError},
		{name: "automergent tool", err: New(CodeToolExecFailed, "tool failed"), want: ExitToolExecutionError},
		{name: "automergent unknown category defaults", err: New(CodeInternal, "boom"), want: ExitGeneral},
		{name: "plain invalid args message", err: errors.New("invalid output format \"xml\""), want: ExitInvalidArgs},
		{name: "plain auth message", err: errors.New("api key not set"), want: ExitAuthFailed},
		{name: "plain provider message", err: errors.New("provider unavailable"), want: ExitAPIError},
		{name: "plain context limit message", err: errors.New("context too long"), want: ExitContextExceeded},
		{name: "plain tool message", err: errors.New("tool failed with exit code 1"), want: ExitToolExecutionError},
		{name: "plain error defaults", err: errors.New("plain failure"), want: ExitGeneral},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExitCodeForError(tt.err)
			if got != tt.want {
				t.Fatalf("ExitCodeForError(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

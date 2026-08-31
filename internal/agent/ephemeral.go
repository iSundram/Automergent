package agent

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
)

// Ephemeral, turn-scoped context injection.
//
// Loop nudges (explore coverage, anti-stall, goal continuation, step-budget
// wrap-up, truncation recovery) must reach the model on exactly the next
// provider call — but persisting them as session messages pollutes the
// history forever: they survive into later turns, get re-sent on every
// request, and leak into compaction summaries as if they were real
// conversation. This queue is the inverse: messages are appended to the
// next request and never written to the session or transcript.
//
// The messages ride as <system-reminder> system messages, the same channel
// the reference agent uses for machine-injected context; the platform prompt
// teaches the model that these tags carry system information, not user
// instructions.

// ephemeralReminderTag wraps ephemeral nudge text so the model can
// distinguish machine-injected context from user-authored messages.
const ephemeralReminderTag = "system-reminder"

// injectEphemeral queues a turn-scoped system reminder for the next provider
// call. The text is wrapped in <system-reminder> tags.
func (a *Agent) injectEphemeral(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	a.injectEphemeralMessage(ai.Message{
		Role: ai.RoleSystem,
		Content: []ai.ContentPart{{
			Type: ai.ContentTypeText,
			Text: fmt.Sprintf("<%s>\n%s\n</%s>", ephemeralReminderTag, strings.TrimSpace(text), ephemeralReminderTag),
		}},
		Metadata: map[string]any{"ephemeral": true},
	})
}

// injectEphemeralMessage queues a pre-built message for the next provider
// call without persisting it.
func (a *Agent) injectEphemeralMessage(msg ai.Message) {
	if msg.Metadata == nil {
		msg.Metadata = map[string]any{"ephemeral": true}
	} else {
		msg.Metadata["ephemeral"] = true
	}
	a.mu.Lock()
	a.ephemeral = append(a.ephemeral, msg)
	a.mu.Unlock()
}

// drainEphemeral returns the queued ephemeral messages and clears the queue.
// The caller appends them to the request-only message projection; they are
// never added to the session.
func (a *Agent) drainEphemeral() []ai.Message {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.ephemeral) == 0 {
		return nil
	}
	msgs := a.ephemeral
	a.ephemeral = nil
	return msgs
}

// appendEphemeral appends the currently queued ephemeral messages to msgs
// (request projection only) and clears the queue.
func (a *Agent) appendEphemeral(msgs []ai.Message) []ai.Message {
	extra := a.drainEphemeral()
	if len(extra) == 0 {
		return msgs
	}
	return append(msgs, extra...)
}

// ephemeralReminderText builds the wrapped reminder text for a nudge.
func ephemeralReminderText(label, body string) string {
	return fmt.Sprintf("[%s] %s", label, body)
}

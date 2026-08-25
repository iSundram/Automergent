package app

// Message queueing while a run is in flight.
//
// Before this, Enter was gated on `!a.thinking`: typing while the agent worked
// did nothing at all, silently. Now a message typed mid-run is queued and
// delivered when the turn ends — or, with ctrl+j, injected at the next tool
// boundary so it reaches the model without waiting for the whole turn.

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// queuedMessage is one message waiting to be delivered to the agent.
type queuedMessage struct {
	text string
	at   time.Time
	// boundary requests delivery at the next tool boundary of the running turn
	// rather than after it completes.
	boundary bool
	// isCmd marks a slash command, which dispatches locally instead of
	// starting an agent turn.
	isCmd bool
}

// enqueueMessage queues a message typed while the agent is running. Reports
// false when there was nothing to queue.
func (a *App) enqueueMessage(text string, boundary bool) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	msg := queuedMessage{
		text:     text,
		at:       time.Now(),
		boundary: boundary,
		isCmd:    strings.HasPrefix(text, "/"),
	}
	a.msgQueue = append(a.msgQueue, msg)

	// Slash commands are local dispatch, so they cannot be steered into the
	// model's context — they always wait for the turn to end.
	if boundary && !msg.isCmd {
		a.deliverBoundaryMessages()
	}
	return true
}

// deliverBoundaryMessages hands boundary-marked prompts to the agent's steer
// channel. Items the agent cannot accept stay queued for end-of-turn delivery,
// so a message is never dropped.
func (a *App) deliverBoundaryMessages() {
	if a.ag == nil || !a.thinking {
		return
	}
	kept := a.msgQueue[:0]
	for _, msg := range a.msgQueue {
		if msg.boundary && !msg.isCmd && a.ag.Steer(msg.text) {
			a.conversation.AddMessage("system", "↳ steering: "+msg.text, false)
			continue
		}
		kept = append(kept, msg)
	}
	a.msgQueue = kept
}

// markQueueBoundary promotes queued messages to boundary delivery — the ctrl+j
// action. When the input holds text, that text is queued as a boundary message
// instead, so ctrl+j works as "send this now" in one keystroke.
func (a *App) markQueueBoundary() bool {
	if pending := strings.TrimSpace(a.input.Value()); pending != "" {
		a.input.Reset()
		a.updateActiveTokens()
		return a.enqueueMessage(pending, true)
	}
	if len(a.msgQueue) == 0 {
		return false
	}
	for i := range a.msgQueue {
		a.msgQueue[i].boundary = true
	}
	a.deliverBoundaryMessages()
	return true
}

// clearQueue discards every queued message, including any already handed to the
// agent's steer channel.
func (a *App) clearQueue() int {
	n := len(a.msgQueue)
	a.msgQueue = nil
	if a.ag != nil {
		a.ag.ClearSteerQueue()
	}
	return n
}

// drainQueue delivers the next queued message once a run has finished. Returns
// the command that starts it, or nil when the queue is empty.
//
// Only one message is delivered per turn: the rest stay queued so that a reply
// to the first can inform whether the others still make sense.
func (a *App) drainQueue() tea.Cmd {
	for len(a.msgQueue) > 0 {
		msg := a.msgQueue[0]
		a.msgQueue = a.msgQueue[1:]

		text := strings.TrimSpace(msg.text)
		if text == "" {
			continue
		}
		if msg.isCmd {
			return a.handleSlashCommand(text)
		}
		return a.startAgent(text)
	}
	return nil
}

// queuedCount reports how many messages are waiting.
func (a *App) queuedCount() int { return len(a.msgQueue) }

package commands

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// /error (alias /errors) — show recorded API errors and retries.

func errorsCommand() Command {
	return Command{
		Name:             "error",
		Aliases:          []string{"errors"},
		Description:      "Show recorded API errors and retries",
		Category:         "System",
		Icon:             "󰀦",
		ArgsHint:         "[clear|<n>]",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Errors",
		Immediate:        true,
		SupportsHeadless: true,
		WhenToUse:        "After a request failed or took unusually long",
	}
}

// handleErrors renders the recorded provider API failures.
//
// Both retried attempts and terminal failures are recorded, so this doubles as
// an explanation of why a request took a long time: ten retry lines against one
// request is a rate limit, not a hang.
func handleErrors(host Host, args []string) Result {
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "clear":
			host.ClearAPIErrors()
			host.AddSystemMessage("API error history cleared.")
			host.SetStatus("Errors cleared")
			return Done(nil)
		default:
			index, err := strconv.Atoi(args[0])
			if err != nil {
				host.CommandUsage("/error [clear|<n>]")
				return Done(nil)
			}
			showErrorDetail(host, index)
			return Done(nil)
		}
	}

	records := host.APIErrors()
	if len(records) == 0 {
		host.AddSystemMessage("No API errors recorded this session.")
		host.SetStatus("No errors")
		return Done(nil)
	}

	var b strings.Builder
	retried, terminal := countErrors(records)
	fmt.Fprintf(&b, "API errors this session: %d (%d retried, %d final)\n\n",
		len(records), retried, terminal)

	for i, rec := range records {
		// 1-based, newest first — the same numbering /error <n> accepts.
		fmt.Fprintf(&b, "%2d. %s", i+1, rec.Code)
		if rec.Detail != "" {
			fmt.Fprintf(&b, " · %s", rec.Detail)
		}
		if rec.Retrying {
			if rec.MaxAttempts > 0 {
				fmt.Fprintf(&b, " · retry %d/%d", rec.Attempt, rec.MaxAttempts)
			} else {
				b.WriteString(" · retried")
			}
		} else if rec.MaxAttempts > 1 {
			fmt.Fprintf(&b, " · failed after %d attempts", rec.MaxAttempts)
		} else {
			b.WriteString(" · failed")
		}
		fmt.Fprintf(&b, " · %s ago\n", humanAge(time.Since(rec.At)))

		if msg := firstLine(rec.Message); msg != "" {
			fmt.Fprintf(&b, "    %s\n", truncate(msg, 100))
		}
	}

	b.WriteString("\n/error <n> shows one entry in full · /error clear empties the log")
	host.AddSystemMessage(b.String())
	host.SetStatus(fmt.Sprintf("%d API errors", len(records)))
	return Done(nil)
}

// showErrorDetail prints one entry in full, including the provider's own
// remediation suggestion and request id when it supplied them.
func showErrorDetail(host Host, index int) {
	records := host.APIErrors()
	if index < 1 || index > len(records) {
		host.CommandError(fmt.Sprintf("no error %d (have %d)", index, len(records)))
		return
	}
	rec := records[index-1]

	var b strings.Builder
	fmt.Fprintf(&b, "Error %d of %d\n\n", index, len(records))
	fmt.Fprintf(&b, "Code:      %s\n", rec.Code)
	if rec.Detail != "" {
		fmt.Fprintf(&b, "Detail:    %s\n", rec.Detail)
	}
	fmt.Fprintf(&b, "When:      %s (%s ago)\n", rec.At.Format("15:04:05"), humanAge(time.Since(rec.At)))
	if rec.Retrying {
		fmt.Fprintf(&b, "Outcome:   retried (attempt %d of %d)\n", rec.Attempt, rec.MaxAttempts)
	} else {
		fmt.Fprintf(&b, "Outcome:   final failure after %d attempt(s)\n", maxInt(rec.MaxAttempts, 1))
	}
	if rec.Provider != "" {
		fmt.Fprintf(&b, "Provider:  %s", rec.Provider)
		if rec.Model != "" {
			fmt.Fprintf(&b, " · %s", rec.Model)
		}
		b.WriteString("\n")
	}
	if rec.RequestID != "" {
		fmt.Fprintf(&b, "RequestID: %s\n", rec.RequestID)
	}
	if rec.Message != "" {
		fmt.Fprintf(&b, "\n%s\n", rec.Message)
	}
	if rec.Suggestion != "" {
		fmt.Fprintf(&b, "\nSuggestion: %s\n", rec.Suggestion)
	}
	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
}

// countErrors splits records into retried attempts and terminal failures.
func countErrors(records []APIErrorInfo) (retried, terminal int) {
	for _, rec := range records {
		if rec.Retrying {
			retried++
			continue
		}
		terminal++
	}
	return retried, terminal
}

// humanAge renders an age compactly: "12s", "4m", "2h".
func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

// firstLine returns the first non-empty line of a multi-line message.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

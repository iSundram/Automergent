package tools

import (
	"context"
	"fmt"

	"github.com/iSundram/Automergent/internal/shared"
)

// ViolationTool detects and handles policy violations.
type ViolationTool struct {
	violationCounts map[shared.ViolationType]int
}

func NewViolationTool() *ViolationTool {
	return &ViolationTool{
		violationCounts: make(map[shared.ViolationType]int),
	}
}

func (t *ViolationTool) Name() string {
	return "violation_detected"
}

func (t *ViolationTool) Description() string {
	return "Report a policy violation (hacking, illegal acts, harmful code, credential theft, security bypass). Triggers escalation: warn -> block_imminent -> blocked. User can override with valid justification."
}

func (t *ViolationTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{
				"type":        "string",
				"enum":        []string{"hacking", "illegal", "harmful_code", "credential_theft", "security_bypass", "persistent_violation"},
				"description": "Type of violation detected",
			},
			"severity": map[string]any{
				"type":        "string",
				"enum":        []string{"low", "medium", "high", "critical"},
				"description": "Severity level",
			},
			"user_message": map[string]any{
				"type":        "string",
				"description": "The user's original message that triggered the violation",
			},
			"agent_response": map[string]any{
				"type":        "string",
				"description": "The agent's response before detecting violation",
			},
			"justification": map[string]any{
				"type":        "string",
				"description": "Optional: user's justification for override (e.g., authorized security research)",
			},
		},
		"required": []string{"type", "severity", "user_message", "agent_response"},
	}
}

func (t *ViolationTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	vType := shared.ViolationType(args["type"].(string))
	severity := shared.ViolationSeverity(args["severity"].(string))
	userMsg := args["user_message"].(string)
	_ = args["agent_response"].(string) // agentResp - available for logging
	justification := ""
	if j, ok := args["justification"].(string); ok {
		justification = j
	}

	// Check for override justification
	if justification != "" {
		t.violationCounts[vType] = 0 // Reset count on valid override
		return Result{
			Content: fmt.Sprintf("VIOLATION OVERRIDDEN: %s - User provided justification: %s", vType, justification),
			Metadata: map[string]any{
				"action":       "overridden",
				"violation_type": vType,
				"justification": justification,
			},
		}, nil
	}

	// Increment violation count
	t.violationCounts[vType]++
	count := t.violationCounts[vType]

	var action string
	switch {
	case count == 1:
		action = "warn"
	case count == 2:
		action = "block_imminent"
	case count >= 3:
		action = "blocked"
	}

	var msg string
	switch action {
	case "warn":
		msg = fmt.Sprintf("⚠ VIOLATION DETECTED (%s): %s\nSeverity: %s\nThis is warning #1. If you continue, the session will be blocked.\nIf this is a legitimate request (e.g., authorized security research), provide justification to override.", vType, userMsg, severity)
	case "block_imminent":
		msg = fmt.Sprintf("⚠⚠ SECOND VIOLATION (%s): %s\nSeverity: %s\nThis is warning #2. The session will be BLOCKED on the next violation.\nProvide justification now if this is legitimate.", vType, userMsg, severity)
	case "blocked":
		msg = fmt.Sprintf("🚫 SESSION BLOCKED: Persistent violations (%s)\nReason: %s\nSeverity: %s\nThe session has been blocked due to repeated policy violations.", vType, userMsg, severity)
	}

	return Result{
		Content: msg,
		Metadata: map[string]any{
			"action":         action,
			"violation_type": vType,
			"severity":       severity,
			"count":          count,
			"user_message":   userMsg,
		},
		IsError: action == "blocked",
	}, nil
}

func (t *ViolationTool) IsReadOnly(args map[string]any) bool {
	return true
}

func (t *ViolationTool) IsDestructive(args map[string]any) bool {
	return false
}

func (t *ViolationTool) IsConcurrencySafe(args map[string]any) bool {
	return true
}

func (t *ViolationTool) RequiresConfirmation(mode string) bool {
	return false
}

func (t *ViolationTool) EstimatedCost() ToolCost {
	return ToolCost{RiskLevel: "low"}
}

// GetViolationCount returns the current violation count for a type.
func (t *ViolationTool) GetViolationCount(vType shared.ViolationType) int {
	return t.violationCounts[vType]
}

// ResetViolationCount resets the violation count for a type.
func (t *ViolationTool) ResetViolationCount(vType shared.ViolationType) {
	t.violationCounts[vType] = 0
}

// BlockSessionTool blocks the current session.
type BlockSessionTool struct{}

func NewBlockSessionTool() *BlockSessionTool {
	return &BlockSessionTool{}
}

func (t *BlockSessionTool) Name() string {
	return "block_session"
}

func (t *BlockSessionTool) Description() string {
	return "Block the current session due to persistent policy violations. Use after violation_detected returns 'blocked' action."
}

func (t *BlockSessionTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reason": map[string]any{
				"type":        "string",
				"description": "Reason for blocking the session",
			},
		},
		"required": []string{"reason"},
	}
}

func (t *BlockSessionTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	reason := args["reason"].(string)
	return Result{
		Content: fmt.Sprintf("🚫 SESSION BLOCKED: %s", reason),
		Metadata: map[string]any{
			"blocked": true,
			"reason":  reason,
		},
		IsError: true,
	}, nil
}

func (t *BlockSessionTool) IsReadOnly(args map[string]any) bool {
	return true
}

func (t *BlockSessionTool) IsDestructive(args map[string]any) bool {
	return false
}

func (t *BlockSessionTool) IsConcurrencySafe(args map[string]any) bool {
	return true
}

func (t *BlockSessionTool) RequiresConfirmation(mode string) bool {
	return false
}

func (t *BlockSessionTool) EstimatedCost() ToolCost {
	return ToolCost{RiskLevel: "low"}
}

// OverrideViolationTool allows user to override a violation with justification.
type OverrideViolationTool struct{}

func NewOverrideViolationTool() *OverrideViolationTool {
	return &OverrideViolationTool{}
}

func (t *OverrideViolationTool) Name() string {
	return "override_violation"
}

func (t *OverrideViolationTool) Description() string {
	return "Override a violation detection with user justification (e.g., authorized security research, educational purpose). Resets violation count."
}

func (t *OverrideViolationTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"violation_type": map[string]any{
				"type":        "string",
				"enum":        []string{"hacking", "illegal", "harmful_code", "credential_theft", "security_bypass", "persistent_violation"},
				"description": "Type of violation to override",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "User's justification for the override",
			},
		},
		"required": []string{"violation_type", "reason"},
	}
}

func (t *OverrideViolationTool) Execute(ctx context.Context, args map[string]any) (Result, error) {
	vType := shared.ViolationType(args["violation_type"].(string))
	reason := args["reason"].(string)

	return Result{
		Content: fmt.Sprintf("✓ VIOLATION OVERRIDDEN: %s\nReason: %s\nViolation count reset. Continuing...", vType, reason),
		Metadata: map[string]any{
			"overridden":      true,
			"violation_type":  vType,
			"reason":          reason,
		},
	}, nil
}

func (t *OverrideViolationTool) IsReadOnly(args map[string]any) bool {
	return true
}

func (t *OverrideViolationTool) IsDestructive(args map[string]any) bool {
	return false
}

func (t *OverrideViolationTool) IsConcurrencySafe(args map[string]any) bool {
	return true
}

func (t *OverrideViolationTool) RequiresConfirmation(mode string) bool {
	return false
}

func (t *OverrideViolationTool) EstimatedCost() ToolCost {
	return ToolCost{RiskLevel: "low"}
}
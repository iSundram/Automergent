package agent

// Mode defines the approval mode for the agent.
type Mode string

const (
	ModeEdit Mode = "edit"
	ModePlan Mode = "plan"
)

// RequiresConfirmation reports whether the mode requires user confirmation
// for the given operation type.
func (m Mode) RequiresConfirmation(op string) bool {
	switch m {
	case ModeEdit:
		return op == "shell" || op == "write"
	case ModePlan:
		return true
	default:
		return true
	}
}

// IsValid reports whether the mode string is recognized.
func IsValid(mode string) bool {
	switch Mode(mode) {
	case ModeEdit, ModePlan:
		return true
	default:
		return false
	}
}

// AllModes returns all valid mode strings.
func AllModes() []string {
	return []string{
		string(ModeEdit),
		string(ModePlan),
	}
}

package agent

// Mode defines the approval mode for the agent.
type Mode string

const (
	ModeSuggest  Mode = "suggest"
	ModeAutoEdit Mode = "auto-edit"
	ModeFullAuto Mode = "full-auto"
	ModePlan     Mode = "plan"
)

// RequiresConfirmation reports whether the mode requires user confirmation
// for the given operation type.
func (m Mode) RequiresConfirmation(op string) bool {
	switch m {
	case ModeSuggest:
		return true
	case ModeAutoEdit:
		return op == "shell"
	case ModeFullAuto:
		return false
	case ModePlan:
		return true
	default:
		return true
	}
}

// IsValid reports whether the mode string is recognized.
func IsValid(mode string) bool {
	switch Mode(mode) {
	case ModeSuggest, ModeAutoEdit, ModeFullAuto, ModePlan:
		return true
	default:
		return false
	}
}

// AllModes returns all valid mode strings.
func AllModes() []string {
	return []string{
		string(ModeSuggest),
		string(ModeAutoEdit),
		string(ModeFullAuto),
		string(ModePlan),
	}
}

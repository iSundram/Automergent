package accessibility

import (
	"fmt"
	"sort"
	"strings"
)

// KeyBinding represents a keyboard shortcut
type KeyBinding struct {
	Key         string   // Key combination (e.g., "ctrl+s")
	Action      string   // Action ID (e.g., "save")
	Description string   // Human-readable description
	Context     string   // Context where binding is active (e.g., "global", "input", "diff")
	Category    string   // Category for grouping in help (e.g., "Navigation", "Editing")
	Enabled     bool     // Whether binding is currently enabled
	Hidden      bool     // Whether to hide from help overlay
	Aliases     []string // Alternative key combinations
}

// KeyboardManager handles keyboard navigation and shortcuts
type KeyboardManager struct {
	bindings     map[string][]KeyBinding // key -> bindings (multiple bindings per key for different contexts)
	actionMap    map[string]KeyBinding   // action -> primary binding
	contexts     []string                // stack of active contexts
	defaultCtx   string                  // default context
	helpVisible  bool
	focusTrapped bool // whether focus is trapped in a modal

	// Callbacks
	onHelp     func()
	onShortcut func(binding KeyBinding)
}

// NewKeyboardManager creates a new keyboard manager
func NewKeyboardManager() *KeyboardManager {
	return &KeyboardManager{
		bindings:   make(map[string][]KeyBinding),
		actionMap:  make(map[string]KeyBinding),
		contexts:   []string{"global"},
		defaultCtx: "global",
	}
}

// RegisterBinding registers a keyboard shortcut
func (km *KeyboardManager) RegisterBinding(binding KeyBinding) {
	if !binding.Enabled {
		binding.Enabled = true
	}

	// Add to key map
	key := normalizeKey(binding.Key)
	km.bindings[key] = append(km.bindings[key], binding)

	// Add aliases
	for _, alias := range binding.Aliases {
		aliasKey := normalizeKey(alias)
		km.bindings[aliasKey] = append(km.bindings[aliasKey], binding)
	}

	// Add to action map
	km.actionMap[binding.Action] = binding
}

// RegisterBindings registers multiple keyboard shortcuts
func (km *KeyboardManager) RegisterBindings(bindings []KeyBinding) {
	for _, b := range bindings {
		km.RegisterBinding(b)
	}
}

// UnregisterBinding removes a keyboard shortcut
func (km *KeyboardManager) UnregisterBinding(action string) {
	binding, ok := km.actionMap[action]
	if !ok {
		return
	}

	// Remove from key map
	key := normalizeKey(binding.Key)
	bindings := km.bindings[key]
	for i, b := range bindings {
		if b.Action == action {
			km.bindings[key] = append(bindings[:i], bindings[i+1:]...)
			break
		}
	}

	// Remove aliases
	for _, alias := range binding.Aliases {
		aliasKey := normalizeKey(alias)
		aliasBindings := km.bindings[aliasKey]
		for i, b := range aliasBindings {
			if b.Action == action {
				km.bindings[aliasKey] = append(aliasBindings[:i], aliasBindings[i+1:]...)
				break
			}
		}
	}

	// Remove from action map
	delete(km.actionMap, action)
}

// SetEnabled enables or disables a binding
func (km *KeyboardManager) SetEnabled(action string, enabled bool) {
	if binding, ok := km.actionMap[action]; ok {
		binding.Enabled = enabled
		km.actionMap[action] = binding
	}
}

// PushContext pushes a new context onto the stack
func (km *KeyboardManager) PushContext(ctx string) {
	km.contexts = append(km.contexts, ctx)
}

// PopContext removes the top context from the stack
func (km *KeyboardManager) PopContext() string {
	if len(km.contexts) <= 1 {
		return km.defaultCtx
	}
	ctx := km.contexts[len(km.contexts)-1]
	km.contexts = km.contexts[:len(km.contexts)-1]
	return ctx
}

// CurrentContext returns the current context
func (km *KeyboardManager) CurrentContext() string {
	if len(km.contexts) == 0 {
		return km.defaultCtx
	}
	return km.contexts[len(km.contexts)-1]
}

// SetFocusTrapped sets whether focus is trapped in a modal
func (km *KeyboardManager) SetFocusTrapped(trapped bool) {
	km.focusTrapped = trapped
}

// IsFocusTrapped returns whether focus is trapped
func (km *KeyboardManager) IsFocusTrapped() bool {
	return km.focusTrapped
}

// Lookup finds the binding for a key in the current context
func (km *KeyboardManager) Lookup(key string) (KeyBinding, bool) {
	normalizedKey := normalizeKey(key)
	bindings, ok := km.bindings[normalizedKey]
	if !ok || len(bindings) == 0 {
		return KeyBinding{}, false
	}

	currentCtx := km.CurrentContext()

	// First, look for exact context match
	for _, b := range bindings {
		if b.Enabled && b.Context == currentCtx {
			return b, true
		}
	}

	// Then, look for global context (unless focus is trapped)
	if !km.focusTrapped {
		for _, b := range bindings {
			if b.Enabled && b.Context == "global" {
				return b, true
			}
		}
	}

	return KeyBinding{}, false
}

// GetBinding returns the binding for an action
func (km *KeyboardManager) GetBinding(action string) (KeyBinding, bool) {
	binding, ok := km.actionMap[action]
	return binding, ok
}

// GetBindingsForContext returns all bindings for a context
func (km *KeyboardManager) GetBindingsForContext(ctx string) []KeyBinding {
	var result []KeyBinding
	seen := make(map[string]bool)

	for _, bindings := range km.bindings {
		for _, b := range bindings {
			if b.Context == ctx && !seen[b.Action] {
				result = append(result, b)
				seen[b.Action] = true
			}
		}
	}

	return result
}

// GetAllBindings returns all unique bindings
func (km *KeyboardManager) GetAllBindings() []KeyBinding {
	var result []KeyBinding
	for _, binding := range km.actionMap {
		result = append(result, binding)
	}
	return result
}

// GetBindingsByCategory returns bindings grouped by category
func (km *KeyboardManager) GetBindingsByCategory() map[string][]KeyBinding {
	categories := make(map[string][]KeyBinding)

	for _, binding := range km.actionMap {
		if binding.Hidden {
			continue
		}
		cat := binding.Category
		if cat == "" {
			cat = "General"
		}
		categories[cat] = append(categories[cat], binding)
	}

	// Sort bindings within each category by key
	for cat := range categories {
		sort.Slice(categories[cat], func(i, j int) bool {
			return categories[cat][i].Key < categories[cat][j].Key
		})
	}

	return categories
}

// FormatHelp returns formatted help text for all bindings
func (km *KeyboardManager) FormatHelp() string {
	categories := km.GetBindingsByCategory()

	// Sort category names
	var catNames []string
	for cat := range categories {
		catNames = append(catNames, cat)
	}
	sort.Strings(catNames)

	var sb strings.Builder
	sb.WriteString("Keyboard Shortcuts\n")
	sb.WriteString("==================\n\n")

	for _, cat := range catNames {
		bindings := categories[cat]
		sb.WriteString(fmt.Sprintf("%s:\n", cat))

		maxKeyLen := 0
		for _, b := range bindings {
			if len(b.Key) > maxKeyLen {
				maxKeyLen = len(b.Key)
			}
		}

		for _, b := range bindings {
			if !b.Enabled {
				continue
			}
			padding := strings.Repeat(" ", maxKeyLen-len(b.Key)+2)
			sb.WriteString(fmt.Sprintf("  %s%s%s\n", b.Key, padding, b.Description))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// SetOnHelp sets the callback for when help is requested
func (km *KeyboardManager) SetOnHelp(fn func()) {
	km.onHelp = fn
}

// SetOnShortcut sets the callback for when a shortcut is used
func (km *KeyboardManager) SetOnShortcut(fn func(KeyBinding)) {
	km.onShortcut = fn
}

// TriggerHelp shows the help overlay
func (km *KeyboardManager) TriggerHelp() {
	km.helpVisible = true
	if km.onHelp != nil {
		km.onHelp()
	}
}

// HideHelp hides the help overlay
func (km *KeyboardManager) HideHelp() {
	km.helpVisible = false
}

// IsHelpVisible returns whether help is visible
func (km *KeyboardManager) IsHelpVisible() bool {
	return km.helpVisible
}

// normalizeKey normalizes a key string for consistent lookup
func normalizeKey(key string) string {
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, " ", "")
	// Normalize modifier order: ctrl+alt+shift+key
	parts := strings.Split(key, "+")
	if len(parts) <= 1 {
		return key
	}

	var ctrl, alt, shift, meta bool
	var mainKey string

	for _, part := range parts {
		switch part {
		case "ctrl", "control":
			ctrl = true
		case "alt", "option":
			alt = true
		case "shift":
			shift = true
		case "meta", "cmd", "command", "win", "super":
			meta = true
		default:
			mainKey = part
		}
	}

	var result []string
	if ctrl {
		result = append(result, "ctrl")
	}
	if alt {
		result = append(result, "alt")
	}
	if shift {
		result = append(result, "shift")
	}
	if meta {
		result = append(result, "meta")
	}
	result = append(result, mainKey)

	return strings.Join(result, "+")
}

// DefaultBindings returns the default keyboard bindings for the TUI
func DefaultBindings() []KeyBinding {
	return []KeyBinding{
		// Global Navigation
		{Key: "tab", Action: "focus-next", Description: "Move to next pane", Context: "global", Category: "Navigation"},
		{Key: "shift+tab", Action: "focus-prev", Description: "Move to previous pane", Context: "global", Category: "Navigation"},
		{Key: "?", Action: "show-help", Description: "Show help overlay", Context: "global", Category: "Navigation"},
		{Key: "esc", Action: "close-modal", Description: "Close modal/cancel", Context: "global", Category: "Navigation"},

		// Application
		{Key: "ctrl+q", Action: "quit", Description: "Quit immediately", Context: "global", Category: "Application"},
		{Key: "ctrl+c", Action: "interrupt", Description: "Cancel/interrupt", Context: "global", Category: "Application"},
		{Key: "ctrl+s", Action: "open-sessions", Description: "Open sessions", Context: "global", Category: "Application"},

		// Panes
		{Key: "ctrl+d", Action: "toggle-diff", Description: "Toggle diff pane", Context: "global", Category: "Panes", Aliases: []string{"f2"}},
		{Key: "ctrl+w", Action: "next-diff-tab", Description: "Open modified-files view / next diff tab", Context: "global", Category: "Panes"},
		{Key: "ctrl+t", Action: "toggle-tree", Description: "Toggle file tree", Context: "global", Category: "Panes"},
		{Key: "ctrl+l", Action: "toggle-lsp", Description: "Toggle LSP panel", Context: "global", Category: "Panes"},
		{Key: "ctrl+r", Action: "toggle-review", Description: "Toggle review mode", Context: "global", Category: "Panes"},

		// Input
		{Key: "enter", Action: "send-message", Description: "Send message", Context: "input", Category: "Input"},
		{Key: "ctrl+u", Action: "clear-input", Description: "Clear input", Context: "input", Category: "Input"},
		{Key: "/", Action: "open-palette", Description: "Open command palette", Context: "input", Category: "Input"},
		{Key: "@", Action: "open-file-picker", Description: "Open file picker", Context: "input", Category: "Input"},

		// Conversation
		{Key: "up", Action: "scroll-up", Description: "Scroll up", Context: "conversation", Category: "Conversation"},
		{Key: "down", Action: "scroll-down", Description: "Scroll down", Context: "conversation", Category: "Conversation"},
		{Key: "pgup", Action: "page-up", Description: "Page up", Context: "conversation", Category: "Conversation"},
		{Key: "pgdown", Action: "page-down", Description: "Page down", Context: "conversation", Category: "Conversation"},
		{Key: "home", Action: "scroll-top", Description: "Scroll to top", Context: "conversation", Category: "Conversation"},
		{Key: "end", Action: "scroll-bottom", Description: "Scroll to bottom", Context: "conversation", Category: "Conversation"},

		// Diff
		{Key: "n", Action: "next-hunk", Description: "Next hunk", Context: "diff", Category: "Diff"},
		{Key: "p", Action: "prev-hunk", Description: "Previous hunk", Context: "diff", Category: "Diff"},
		{Key: "j", Action: "diff-down", Description: "Scroll down", Context: "diff", Category: "Diff"},
		{Key: "k", Action: "diff-up", Description: "Scroll up", Context: "diff", Category: "Diff"},

		// File Tree
		{Key: "up", Action: "tree-up", Description: "Move up", Context: "tree", Category: "File Tree", Aliases: []string{"k"}},
		{Key: "down", Action: "tree-down", Description: "Move down", Context: "tree", Category: "File Tree", Aliases: []string{"j"}},
		{Key: "enter", Action: "tree-select", Description: "Select file", Context: "tree", Category: "File Tree"},
		{Key: "space", Action: "tree-toggle", Description: "Toggle directory", Context: "tree", Category: "File Tree"},

		// Command Palette
		{Key: "up", Action: "palette-up", Description: "Previous item", Context: "palette", Category: "Command Palette"},
		{Key: "down", Action: "palette-down", Description: "Next item", Context: "palette", Category: "Command Palette"},
		{Key: "tab", Action: "palette-complete", Description: "Complete selection", Context: "palette", Category: "Command Palette"},
		{Key: "enter", Action: "palette-select", Description: "Select item", Context: "palette", Category: "Command Palette"},

		// Confirmation Dialog
		{Key: "y", Action: "confirm-yes", Description: "Accept/confirm", Context: "confirm", Category: "Confirmation", Aliases: []string{"Y"}},
		{Key: "n", Action: "confirm-no", Description: "Reject/deny", Context: "confirm", Category: "Confirmation", Aliases: []string{"N"}},

		// Accessibility
		{Key: "ctrl+a", Action: "accessibility-menu", Description: "Open accessibility menu", Context: "global", Category: "Accessibility", Hidden: true},
	}
}

// VimBindings returns vim-style keyboard bindings
func VimBindings() []KeyBinding {
	bindings := DefaultBindings()

	// Add vim-specific bindings
	vimBindings := []KeyBinding{
		{Key: "j", Action: "scroll-down", Description: "Scroll down", Context: "conversation", Category: "Conversation"},
		{Key: "k", Action: "scroll-up", Description: "Scroll up", Context: "conversation", Category: "Conversation"},
		{Key: "g", Action: "scroll-top", Description: "Go to top", Context: "conversation", Category: "Conversation"},
		{Key: "G", Action: "scroll-bottom", Description: "Go to bottom", Context: "conversation", Category: "Conversation"},
		{Key: "ctrl+f", Action: "page-down", Description: "Page down", Context: "conversation", Category: "Conversation"},
		{Key: "ctrl+b", Action: "page-up", Description: "Page up", Context: "conversation", Category: "Conversation"},
	}

	return append(bindings, vimBindings...)
}

// EmacsBindings returns emacs-style keyboard bindings
func EmacsBindings() []KeyBinding {
	bindings := DefaultBindings()

	// Add emacs-specific bindings
	emacsBindings := []KeyBinding{
		{Key: "ctrl+n", Action: "scroll-down", Description: "Next line", Context: "conversation", Category: "Conversation"},
		{Key: "ctrl+p", Action: "scroll-up", Description: "Previous line", Context: "conversation", Category: "Conversation"},
		{Key: "meta+<", Action: "scroll-top", Description: "Beginning of buffer", Context: "conversation", Category: "Conversation"},
		{Key: "meta+>", Action: "scroll-bottom", Description: "End of buffer", Context: "conversation", Category: "Conversation"},
		{Key: "ctrl+v", Action: "page-down", Description: "Scroll down page", Context: "conversation", Category: "Conversation"},
		{Key: "meta+v", Action: "page-up", Description: "Scroll up page", Context: "conversation", Category: "Conversation"},
	}

	return append(bindings, emacsBindings...)
}

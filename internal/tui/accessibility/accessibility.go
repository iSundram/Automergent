// Package accessibility provides screen reader support, keyboard navigation,
// and visual accessibility features for the TUI system.
package accessibility

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Priority levels for announcements (similar to ARIA live regions)
type AnnouncementPriority int

const (
	// PriorityPolite waits for the user to pause before announcing
	PriorityPolite AnnouncementPriority = iota
	// PriorityAssertive interrupts current speech
	PriorityAssertive
	// PriorityCritical highest priority, for errors and critical information
	PriorityCritical
)

// String returns a human-readable priority name
func (p AnnouncementPriority) String() string {
	switch p {
	case PriorityPolite:
		return "polite"
	case PriorityAssertive:
		return "assertive"
	case PriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// Announcement represents a message to be read by a screen reader
type Announcement struct {
	Text      string
	Priority  AnnouncementPriority
	Role      string // semantic role: "alert", "status", "log", "navigation"
	Timestamp time.Time
}

// FocusInfo describes the current focus state
type FocusInfo struct {
	Component   string // component ID
	Label       string // human-readable label
	Description string // additional context
	Index       int    // position in focus order
	Total       int    // total focusable items
	Role        string // semantic role
}

// ComponentInfo provides accessibility metadata for a component
type ComponentInfo struct {
	ID          string   // unique component identifier
	Label       string   // screen reader label
	Description string   // detailed description
	Role        string   // semantic role (button, textbox, list, etc.)
	State       string   // current state (expanded, selected, etc.)
	Value       string   // current value if applicable
	Shortcuts   []string // available keyboard shortcuts
	Children    int      // number of child items
	Position    int      // current position in list (1-indexed)
	Total       int      // total items in list
}

// String returns a screen reader friendly description
func (c *ComponentInfo) String() string {
	var parts []string

	if c.Label != "" {
		parts = append(parts, c.Label)
	}
	if c.Role != "" {
		parts = append(parts, c.Role)
	}
	if c.State != "" {
		parts = append(parts, c.State)
	}
	if c.Position > 0 && c.Total > 0 {
		parts = append(parts, fmt.Sprintf("%d of %d", c.Position, c.Total))
	}
	if c.Value != "" {
		parts = append(parts, c.Value)
	}
	if c.Description != "" {
		parts = append(parts, c.Description)
	}

	return strings.Join(parts, ", ")
}

// Manager handles all accessibility features
type Manager struct {
	mu sync.RWMutex

	// Screen reader support
	enabled       bool
	announcements []Announcement
	maxHistory    int

	// Focus management
	currentFocus  FocusInfo
	focusOrder    []string
	focusLabels   map[string]string
	focusHandlers map[string]func() ComponentInfo

	// Visual accessibility
	highContrast   bool
	reducedMotion  bool
	largeText      bool
	colorBlindMode string // "none", "protanopia", "deuteranopia", "tritanopia"

	// Audio feedback
	audioEnabled bool
	audioVolume  float64
	soundEffects map[string]bool

	// Internationalization
	locale       string
	rtlMode      bool
	translations map[string]map[string]string

	// Callbacks
	onAnnounce func(Announcement)
	onFocus    func(FocusInfo)
}

// NewManager creates a new accessibility manager
func NewManager() *Manager {
	return &Manager{
		enabled:        true,
		maxHistory:     100,
		announcements:  make([]Announcement, 0, 100),
		focusOrder:     make([]string, 0),
		focusLabels:    make(map[string]string),
		focusHandlers:  make(map[string]func() ComponentInfo),
		soundEffects:   make(map[string]bool),
		audioVolume:    1.0,
		locale:         "en",
		translations:   make(map[string]map[string]string),
		colorBlindMode: "none",
	}
}

// SetEnabled enables or disables accessibility features
func (m *Manager) SetEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
}

// IsEnabled returns whether accessibility features are enabled
func (m *Manager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// Announce adds a new announcement to the queue
func (m *Manager) Announce(text string, priority AnnouncementPriority) {
	m.AnnounceWithRole(text, priority, "status")
}

// AnnounceWithRole adds an announcement with a specific semantic role
func (m *Manager) AnnounceWithRole(text string, priority AnnouncementPriority, role string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.enabled || text == "" {
		return
	}

	ann := Announcement{
		Text:      text,
		Priority:  priority,
		Role:      role,
		Timestamp: time.Now(),
	}

	m.announcements = append(m.announcements, ann)

	// Trim history
	if len(m.announcements) > m.maxHistory {
		m.announcements = m.announcements[len(m.announcements)-m.maxHistory:]
	}

	// Trigger callback
	if m.onAnnounce != nil {
		go m.onAnnounce(ann)
	}
}

// Alert is a convenience method for critical announcements
func (m *Manager) Alert(text string) {
	m.AnnounceWithRole(text, PriorityCritical, "alert")
}

// Status is a convenience method for status updates
func (m *Manager) Status(text string) {
	m.AnnounceWithRole(text, PriorityPolite, "status")
}

// Log is a convenience method for log messages
func (m *Manager) Log(text string) {
	m.AnnounceWithRole(text, PriorityPolite, "log")
}

// GetAnnouncements returns recent announcements
func (m *Manager) GetAnnouncements(limit int) []Announcement {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.announcements) {
		limit = len(m.announcements)
	}

	start := len(m.announcements) - limit
	result := make([]Announcement, limit)
	copy(result, m.announcements[start:])
	return result
}

// ClearAnnouncements clears the announcement history
func (m *Manager) ClearAnnouncements() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.announcements = m.announcements[:0]
}

// SetOnAnnounce sets a callback for new announcements
func (m *Manager) SetOnAnnounce(fn func(Announcement)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAnnounce = fn
}

// SetOnFocus sets a callback for focus changes
func (m *Manager) SetOnFocus(fn func(FocusInfo)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onFocus = fn
}

// RegisterComponent registers a component with the accessibility system
func (m *Manager) RegisterComponent(id, label string, handler func() ComponentInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.focusLabels[id] = label
	if handler != nil {
		m.focusHandlers[id] = handler
	}
}

// SetFocusOrder sets the tab order for components
func (m *Manager) SetFocusOrder(order []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.focusOrder = make([]string, len(order))
	copy(m.focusOrder, order)
}

// GetFocusOrder returns the current focus order
func (m *Manager) GetFocusOrder() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]string, len(m.focusOrder))
	copy(result, m.focusOrder)
	return result
}

// SetFocus updates the current focus and announces the change
func (m *Manager) SetFocus(componentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.enabled {
		return
	}

	label := m.focusLabels[componentID]
	if label == "" {
		label = componentID
	}

	index := -1
	for i, id := range m.focusOrder {
		if id == componentID {
			index = i + 1
			break
		}
	}

	info := FocusInfo{
		Component: componentID,
		Label:     label,
		Index:     index,
		Total:     len(m.focusOrder),
	}

	// Get detailed component info if available
	if handler, ok := m.focusHandlers[componentID]; ok {
		compInfo := handler()
		info.Description = compInfo.Description
		info.Role = compInfo.Role
	}

	oldFocus := m.currentFocus.Component
	m.currentFocus = info

	// Announce focus change
	if oldFocus != componentID && m.onAnnounce != nil {
		ann := Announcement{
			Text:      fmt.Sprintf("%s, %s", info.Label, info.Role),
			Priority:  PriorityAssertive,
			Role:      "navigation",
			Timestamp: time.Now(),
		}
		go m.onAnnounce(ann)
	}

	// Trigger focus callback
	if m.onFocus != nil {
		go m.onFocus(info)
	}
}

// GetFocus returns the current focus information
func (m *Manager) GetFocus() FocusInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentFocus
}

// NextFocus moves focus to the next component
func (m *Manager) NextFocus() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.focusOrder) == 0 {
		return ""
	}

	currentIdx := -1
	for i, id := range m.focusOrder {
		if id == m.currentFocus.Component {
			currentIdx = i
			break
		}
	}

	nextIdx := (currentIdx + 1) % len(m.focusOrder)
	return m.focusOrder[nextIdx]
}

// PreviousFocus moves focus to the previous component
func (m *Manager) PreviousFocus() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.focusOrder) == 0 {
		return ""
	}

	currentIdx := 0
	for i, id := range m.focusOrder {
		if id == m.currentFocus.Component {
			currentIdx = i
			break
		}
	}

	prevIdx := (currentIdx - 1 + len(m.focusOrder)) % len(m.focusOrder)
	return m.focusOrder[prevIdx]
}

// GetComponentInfo returns accessibility info for a component
func (m *Manager) GetComponentInfo(componentID string) ComponentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if handler, ok := m.focusHandlers[componentID]; ok {
		return handler()
	}

	return ComponentInfo{
		ID:    componentID,
		Label: m.focusLabels[componentID],
	}
}

// Visual Accessibility Methods

// SetHighContrast enables or disables high contrast mode
func (m *Manager) SetHighContrast(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.highContrast = enabled
}

// IsHighContrast returns whether high contrast mode is enabled
func (m *Manager) IsHighContrast() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.highContrast
}

// SetReducedMotion enables or disables reduced motion
func (m *Manager) SetReducedMotion(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reducedMotion = enabled
}

// IsReducedMotion returns whether reduced motion is enabled
func (m *Manager) IsReducedMotion() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.reducedMotion
}

// SetLargeText enables or disables large text mode
func (m *Manager) SetLargeText(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.largeText = enabled
}

// IsLargeText returns whether large text mode is enabled
func (m *Manager) IsLargeText() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.largeText
}

// SetColorBlindMode sets the color blind accommodation mode
func (m *Manager) SetColorBlindMode(mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.colorBlindMode = mode
}

// GetColorBlindMode returns the current color blind mode
func (m *Manager) GetColorBlindMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.colorBlindMode
}

// Audio Feedback Methods

// SetAudioEnabled enables or disables audio feedback
func (m *Manager) SetAudioEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audioEnabled = enabled
}

// IsAudioEnabled returns whether audio feedback is enabled
func (m *Manager) IsAudioEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.audioEnabled
}

// SetAudioVolume sets the audio volume (0.0 to 1.0)
func (m *Manager) SetAudioVolume(volume float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if volume < 0 {
		volume = 0
	}
	if volume > 1 {
		volume = 1
	}
	m.audioVolume = volume
}

// GetAudioVolume returns the current audio volume
func (m *Manager) GetAudioVolume() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.audioVolume
}

// EnableSoundEffect enables a specific sound effect
func (m *Manager) EnableSoundEffect(name string, enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.soundEffects[name] = enabled
}

// IsSoundEffectEnabled returns whether a sound effect is enabled
func (m *Manager) IsSoundEffectEnabled(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	enabled, ok := m.soundEffects[name]
	return ok && enabled
}

// Internationalization Methods

// SetLocale sets the current locale
func (m *Manager) SetLocale(locale string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.locale = locale
	// Auto-detect RTL for common RTL locales
	m.rtlMode = isRTLLocale(locale)
}

// GetLocale returns the current locale
func (m *Manager) GetLocale() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.locale
}

// SetRTLMode enables or disables RTL layout
func (m *Manager) SetRTLMode(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rtlMode = enabled
}

// IsRTLMode returns whether RTL mode is enabled
func (m *Manager) IsRTLMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rtlMode
}

// LoadTranslations loads translations for a locale
func (m *Manager) LoadTranslations(locale string, translations map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.translations[locale] = translations
}

// Translate returns the translated string for a key
func (m *Manager) Translate(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if trans, ok := m.translations[m.locale]; ok {
		if val, ok := trans[key]; ok {
			return val
		}
	}

	// Fallback to English
	if trans, ok := m.translations["en"]; ok {
		if val, ok := trans[key]; ok {
			return val
		}
	}

	return key
}

// TranslateWithArgs returns a formatted translated string
func (m *Manager) TranslateWithArgs(key string, args ...any) string {
	template := m.Translate(key)
	if len(args) > 0 {
		return fmt.Sprintf(template, args...)
	}
	return template
}

// Helper functions

// isRTLLocale checks if a locale uses RTL script
func isRTLLocale(locale string) bool {
	rtlLocales := map[string]bool{
		"ar": true, "ar-SA": true, "ar-EG": true, // Arabic
		"he": true, "he-IL": true, // Hebrew
		"fa": true, "fa-IR": true, // Persian
		"ur": true, "ur-PK": true, // Urdu
		"ps": true, // Pashto
		"sd": true, // Sindhi
		"yi": true, // Yiddish
	}
	return rtlLocales[locale]
}

// DescribeKey returns a human-readable description of a key binding
func DescribeKey(key string) string {
	descriptions := map[string]string{
		"enter":     "Enter key",
		"tab":       "Tab key",
		"esc":       "Escape key",
		"escape":    "Escape key",
		"space":     "Spacebar",
		"up":        "Up arrow",
		"down":      "Down arrow",
		"left":      "Left arrow",
		"right":     "Right arrow",
		"pgup":      "Page Up",
		"pgdown":    "Page Down",
		"home":      "Home key",
		"end":       "End key",
		"backspace": "Backspace",
		"delete":    "Delete key",
		"ctrl+c":    "Control C",
		"ctrl+d":    "Control D",
		"ctrl+l":    "Control L",
		"ctrl+q":    "Control Q",
		"ctrl+r":    "Control R",
		"ctrl+s":    "Control S",
		"ctrl+t":    "Control T",
		"ctrl+u":    "Control U",
		"f1":        "F1 function key",
		"f2":        "F2 function key",
		"?":         "Question mark",
	}

	if desc, ok := descriptions[strings.ToLower(key)]; ok {
		return desc
	}
	return key
}

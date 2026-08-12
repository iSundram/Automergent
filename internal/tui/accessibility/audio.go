package accessibility

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// SoundType represents different types of audio feedback
type SoundType string

const (
	SoundFocus      SoundType = "focus"
	SoundSelect     SoundType = "select"
	SoundError      SoundType = "error"
	SoundSuccess    SoundType = "success"
	SoundWarning    SoundType = "warning"
	SoundNotify     SoundType = "notify"
	SoundType_      SoundType = "type"
	SoundDelete     SoundType = "delete"
	SoundOpen       SoundType = "open"
	SoundClose      SoundType = "close"
	SoundNavigation SoundType = "navigation"
	SoundComplete   SoundType = "complete"
	SoundMessage    SoundType = "message"
)

// SoundEffect represents a sound effect configuration
type SoundEffect struct {
	Type       SoundType
	Frequency  int           // Hz for beep
	Duration   time.Duration // Duration for beep
	Pattern    []int         // Pattern for complex beeps [freq, duration, freq, duration, ...]
	CustomPath string        // Path to custom sound file
	Enabled    bool
}

// AudioManager handles audio feedback
type AudioManager struct {
	mu sync.Mutex

	enabled bool
	volume  float64 // 0.0 to 1.0

	sounds map[SoundType]SoundEffect

	// TTS integration
	screenReader *ScreenReader

	// Platform-specific
	beepCmd string
}

// NewAudioManager creates a new audio manager
func NewAudioManager() *AudioManager {
	am := &AudioManager{
		enabled:      false,
		volume:       0.7,
		sounds:       make(map[SoundType]SoundEffect),
		screenReader: NewScreenReader(),
	}

	// Detect beep command
	am.beepCmd = am.detectBeepCommand()

	// Initialize default sounds
	am.initDefaultSounds()

	return am
}

func (am *AudioManager) detectBeepCommand() string {
	switch runtime.GOOS {
	case "linux":
		// Try various beep commands
		cmds := []string{"beep", "paplay", "aplay", "play"}
		for _, cmd := range cmds {
			if _, err := exec.LookPath(cmd); err == nil {
				return cmd
			}
		}
	case "darwin":
		// macOS uses afplay or say
		if _, err := exec.LookPath("afplay"); err == nil {
			return "afplay"
		}
	case "windows":
		return "powershell"
	}
	return ""
}

func (am *AudioManager) initDefaultSounds() {
	// Define default sound effects using simple beep patterns
	am.sounds = map[SoundType]SoundEffect{
		SoundFocus: {
			Type:      SoundFocus,
			Frequency: 800,
			Duration:  50 * time.Millisecond,
			Enabled:   true,
		},
		SoundSelect: {
			Type:      SoundSelect,
			Frequency: 1000,
			Duration:  100 * time.Millisecond,
			Enabled:   true,
		},
		SoundError: {
			Type:    SoundError,
			Pattern: []int{200, 200, 200, 200}, // Low double beep
			Enabled: true,
		},
		SoundSuccess: {
			Type:    SoundSuccess,
			Pattern: []int{800, 100, 1000, 100, 1200, 100}, // Rising tones
			Enabled: true,
		},
		SoundWarning: {
			Type:    SoundWarning,
			Pattern: []int{600, 150, 600, 150}, // Double beep
			Enabled: true,
		},
		SoundNotify: {
			Type:      SoundNotify,
			Frequency: 1200,
			Duration:  150 * time.Millisecond,
			Enabled:   true,
		},
		SoundType_: {
			Type:      SoundType_,
			Frequency: 1500,
			Duration:  10 * time.Millisecond,
			Enabled:   false, // Disabled by default (can be annoying)
		},
		SoundDelete: {
			Type:      SoundDelete,
			Frequency: 400,
			Duration:  100 * time.Millisecond,
			Enabled:   true,
		},
		SoundOpen: {
			Type:    SoundOpen,
			Pattern: []int{600, 100, 900, 100}, // Rising
			Enabled: true,
		},
		SoundClose: {
			Type:    SoundClose,
			Pattern: []int{900, 100, 600, 100}, // Falling
			Enabled: true,
		},
		SoundNavigation: {
			Type:      SoundNavigation,
			Frequency: 700,
			Duration:  30 * time.Millisecond,
			Enabled:   true,
		},
		SoundComplete: {
			Type:    SoundComplete,
			Pattern: []int{800, 100, 1000, 100, 1200, 150}, // Success fanfare
			Enabled: true,
		},
		SoundMessage: {
			Type:      SoundMessage,
			Frequency: 1100,
			Duration:  200 * time.Millisecond,
			Enabled:   true,
		},
	}
}

// SetEnabled enables or disables audio feedback
func (am *AudioManager) SetEnabled(enabled bool) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.enabled = enabled
}

// IsEnabled returns whether audio feedback is enabled
func (am *AudioManager) IsEnabled() bool {
	am.mu.Lock()
	defer am.mu.Unlock()
	return am.enabled
}

// SetVolume sets the volume (0.0 to 1.0)
func (am *AudioManager) SetVolume(volume float64) {
	am.mu.Lock()
	defer am.mu.Unlock()
	if volume < 0 {
		volume = 0
	}
	if volume > 1 {
		volume = 1
	}
	am.volume = volume
}

// GetVolume returns the current volume
func (am *AudioManager) GetVolume() float64 {
	am.mu.Lock()
	defer am.mu.Unlock()
	return am.volume
}

// EnableSound enables or disables a specific sound
func (am *AudioManager) EnableSound(soundType SoundType, enabled bool) {
	am.mu.Lock()
	defer am.mu.Unlock()
	if sound, ok := am.sounds[soundType]; ok {
		sound.Enabled = enabled
		am.sounds[soundType] = sound
	}
}

// SetCustomSound sets a custom sound file for a sound type
func (am *AudioManager) SetCustomSound(soundType SoundType, path string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	if sound, ok := am.sounds[soundType]; ok {
		sound.CustomPath = path
		am.sounds[soundType] = sound
	}
}

// Play plays a sound effect
func (am *AudioManager) Play(soundType SoundType) {
	am.mu.Lock()
	enabled := am.enabled
	volume := am.volume
	sound, ok := am.sounds[soundType]
	beepCmd := am.beepCmd
	am.mu.Unlock()

	if !enabled || !ok || !sound.Enabled {
		return
	}

	go am.playSound(sound, volume, beepCmd)
}

func (am *AudioManager) playSound(sound SoundEffect, volume float64, beepCmd string) {
	// If custom sound file is specified, play it
	if sound.CustomPath != "" {
		am.playFile(sound.CustomPath, volume, beepCmd)
		return
	}

	// If pattern is specified, play the pattern
	if len(sound.Pattern) > 0 {
		am.playPattern(sound.Pattern, volume, beepCmd)
		return
	}

	// Play simple beep
	am.playBeep(sound.Frequency, sound.Duration, volume, beepCmd)
}

func (am *AudioManager) playBeep(frequency int, duration time.Duration, volume float64, beepCmd string) {
	if beepCmd == "" {
		// Fallback to terminal bell
		fmt.Print("\a")
		return
	}

	var cmd *exec.Cmd

	switch beepCmd {
	case "beep":
		// Linux beep command
		cmd = exec.Command("beep",
			"-f", fmt.Sprintf("%d", frequency),
			"-l", fmt.Sprintf("%d", duration.Milliseconds()))

	case "paplay":
		// PulseAudio - need to generate a sound
		am.playGeneratedSound(frequency, duration, volume)
		return

	case "play":
		// SoX play command
		cmd = exec.Command("play", "-n", "synth",
			fmt.Sprintf("%.3f", duration.Seconds()),
			"sine", fmt.Sprintf("%d", frequency),
			"vol", fmt.Sprintf("%.1f", volume))

	case "afplay":
		// macOS - use system sound or generate
		cmd = exec.Command("afplay", "/System/Library/Sounds/Pop.aiff")

	case "powershell":
		// Windows PowerShell beep
		cmd = exec.Command("powershell", "-Command",
			fmt.Sprintf("[console]::Beep(%d,%d)", frequency, duration.Milliseconds()))
	}

	if cmd != nil {
		cmd.Run()
	}
}

func (am *AudioManager) playPattern(pattern []int, volume float64, beepCmd string) {
	for i := 0; i < len(pattern)-1; i += 2 {
		freq := pattern[i]
		dur := time.Duration(pattern[i+1]) * time.Millisecond
		am.playBeep(freq, dur, volume, beepCmd)
		time.Sleep(50 * time.Millisecond) // Gap between beeps
	}
}

func (am *AudioManager) playFile(path string, volume float64, beepCmd string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("paplay"); err == nil {
			cmd = exec.Command("paplay", path)
		} else if _, err := exec.LookPath("aplay"); err == nil {
			cmd = exec.Command("aplay", path)
		}
	case "darwin":
		cmd = exec.Command("afplay", "-v", fmt.Sprintf("%.1f", volume), path)
	case "windows":
		cmd = exec.Command("powershell", "-Command",
			fmt.Sprintf("(New-Object Media.SoundPlayer '%s').PlaySync()", path))
	}

	if cmd != nil {
		cmd.Run()
	}
}

func (am *AudioManager) playGeneratedSound(frequency int, duration time.Duration, volume float64) {
	// For systems without simple beep, we'd need to generate PCM audio
	// This is a simplified fallback that just uses terminal bell
	fmt.Print("\a")
}

// GetScreenReader returns the screen reader instance
func (am *AudioManager) GetScreenReader() *ScreenReader {
	return am.screenReader
}

// Speak speaks text using the screen reader
func (am *AudioManager) Speak(text string, priority AnnouncementPriority) {
	if am.screenReader != nil {
		am.screenReader.Speak(Announcement{
			Text:      text,
			Priority:  priority,
			Timestamp: time.Now(),
		})
	}
}

// SpeakAndPlay speaks text and plays a sound
func (am *AudioManager) SpeakAndPlay(text string, soundType SoundType) {
	am.Play(soundType)
	am.Speak(text, PriorityPolite)
}

// Stop stops all audio output
func (am *AudioManager) Stop() {
	if am.screenReader != nil {
		am.screenReader.Stop()
	}
}

// AudioCue represents a contextual audio cue
type AudioCue struct {
	Name        string
	Description string
	Sound       SoundType
	Speech      string
	Condition   func() bool
}

// AudioCueManager manages contextual audio cues
type AudioCueManager struct {
	audio *AudioManager
	cues  map[string]AudioCue
	mu    sync.RWMutex
}

// NewAudioCueManager creates a new audio cue manager
func NewAudioCueManager(audio *AudioManager) *AudioCueManager {
	return &AudioCueManager{
		audio: audio,
		cues:  make(map[string]AudioCue),
	}
}

// RegisterCue registers an audio cue
func (acm *AudioCueManager) RegisterCue(cue AudioCue) {
	acm.mu.Lock()
	defer acm.mu.Unlock()
	acm.cues[cue.Name] = cue
}

// TriggerCue triggers an audio cue by name
func (acm *AudioCueManager) TriggerCue(name string) {
	acm.mu.RLock()
	cue, ok := acm.cues[name]
	acm.mu.RUnlock()

	if !ok {
		return
	}

	// Check condition if set
	if cue.Condition != nil && !cue.Condition() {
		return
	}

	// Play sound and/or speak
	if cue.Sound != "" {
		acm.audio.Play(cue.Sound)
	}
	if cue.Speech != "" {
		acm.audio.Speak(cue.Speech, PriorityPolite)
	}
}

// DefaultAudioCues returns the default set of audio cues
func DefaultAudioCues() []AudioCue {
	return []AudioCue{
		{
			Name:        "focus-input",
			Description: "Focus moved to input field",
			Sound:       SoundFocus,
			Speech:      "Input field",
		},
		{
			Name:        "focus-conversation",
			Description: "Focus moved to conversation",
			Sound:       SoundFocus,
			Speech:      "Conversation history",
		},
		{
			Name:        "focus-diff",
			Description: "Focus moved to diff view",
			Sound:       SoundFocus,
			Speech:      "Diff viewer",
		},
		{
			Name:        "focus-tree",
			Description: "Focus moved to file tree",
			Sound:       SoundFocus,
			Speech:      "File tree",
		},
		{
			Name:        "message-sent",
			Description: "Message was sent",
			Sound:       SoundMessage,
			Speech:      "Message sent",
		},
		{
			Name:        "response-received",
			Description: "Response received from assistant",
			Sound:       SoundNotify,
			Speech:      "Response received",
		},
		{
			Name:        "response-complete",
			Description: "Response generation complete",
			Sound:       SoundComplete,
			Speech:      "Response complete",
		},
		{
			Name:        "error",
			Description: "An error occurred",
			Sound:       SoundError,
			Speech:      "Error",
		},
		{
			Name:        "tool-request",
			Description: "Tool requires confirmation",
			Sound:       SoundWarning,
			Speech:      "Tool confirmation required",
		},
		{
			Name:        "tool-approved",
			Description: "Tool was approved",
			Sound:       SoundSuccess,
			Speech:      "Approved",
		},
		{
			Name:        "tool-rejected",
			Description: "Tool was rejected",
			Sound:       SoundDelete,
			Speech:      "Rejected",
		},
		{
			Name:        "modal-open",
			Description: "Modal dialog opened",
			Sound:       SoundOpen,
		},
		{
			Name:        "modal-close",
			Description: "Modal dialog closed",
			Sound:       SoundClose,
		},
		{
			Name:        "navigate",
			Description: "Navigation action",
			Sound:       SoundNavigation,
		},
	}
}

// InitDefaultCues initializes the default audio cues
func (acm *AudioCueManager) InitDefaultCues() {
	for _, cue := range DefaultAudioCues() {
		acm.RegisterCue(cue)
	}
}

// GetCueNames returns all registered cue names
func (acm *AudioCueManager) GetCueNames() []string {
	acm.mu.RLock()
	defer acm.mu.RUnlock()

	names := make([]string, 0, len(acm.cues))
	for name := range acm.cues {
		names = append(names, name)
	}
	return names
}

// DescribeCue returns a description of a cue
func (acm *AudioCueManager) DescribeCue(name string) string {
	acm.mu.RLock()
	defer acm.mu.RUnlock()

	if cue, ok := acm.cues[name]; ok {
		return cue.Description
	}
	return ""
}

// EarconPattern represents a non-verbal audio icon (earcon)
type EarconPattern struct {
	Name    string
	Pattern []int // [freq, duration, freq, duration, ...]
}

// StandardEarcons returns standard earcon patterns
func StandardEarcons() map[string]EarconPattern {
	return map[string]EarconPattern{
		"loading": {
			Name:    "loading",
			Pattern: []int{500, 100, 600, 100, 700, 100, 600, 100}, // Cycling
		},
		"thinking": {
			Name:    "thinking",
			Pattern: []int{400, 200, 500, 200, 400, 200}, // Pulsing
		},
		"working": {
			Name:    "working",
			Pattern: []int{600, 50, 700, 50, 800, 50, 700, 50, 600, 50}, // Rapid
		},
		"attention": {
			Name:    "attention",
			Pattern: []int{1200, 100, 1000, 100, 1200, 100}, // Alert
		},
		"positive": {
			Name:    "positive",
			Pattern: []int{600, 100, 800, 100, 1000, 150}, // Rising
		},
		"negative": {
			Name:    "negative",
			Pattern: []int{400, 100, 300, 100, 200, 200}, // Falling
		},
		"neutral": {
			Name:    "neutral",
			Pattern: []int{600, 150, 600, 150}, // Steady
		},
		"boundary": {
			Name:    "boundary",
			Pattern: []int{200, 50, 200, 50}, // Bump
		},
	}
}

// GenerateSpeechDescription generates a speech description for UI state
func GenerateSpeechDescription(componentType string, state map[string]any) string {
	var parts []string

	// Component type
	switch componentType {
	case "input":
		parts = append(parts, "Input field")
		if val, ok := state["placeholder"].(string); ok && val != "" {
			parts = append(parts, val)
		}
		if val, ok := state["value"].(string); ok && val != "" {
			wordCount := len(strings.Fields(val))
			parts = append(parts, fmt.Sprintf("%d words entered", wordCount))
		}

	case "conversation":
		parts = append(parts, "Conversation history")
		if count, ok := state["messageCount"].(int); ok {
			parts = append(parts, fmt.Sprintf("%d messages", count))
		}
		if atBottom, ok := state["atBottom"].(bool); ok && !atBottom {
			parts = append(parts, "scrolled up, new messages may be below")
		}

	case "diff":
		parts = append(parts, "Diff viewer")
		if hunkCount, ok := state["hunkCount"].(int); ok {
			parts = append(parts, fmt.Sprintf("%d changes", hunkCount))
		}
		if currentHunk, ok := state["currentHunk"].(int); ok {
			if total, ok := state["hunkCount"].(int); ok {
				parts = append(parts, fmt.Sprintf("viewing change %d of %d", currentHunk+1, total))
			}
		}
		if filename, ok := state["filename"].(string); ok {
			parts = append(parts, filename)
		}

	case "filetree":
		parts = append(parts, "File tree")
		if itemCount, ok := state["itemCount"].(int); ok {
			parts = append(parts, fmt.Sprintf("%d items", itemCount))
		}
		if current, ok := state["currentItem"].(string); ok {
			parts = append(parts, current)
		}

	case "confirm":
		parts = append(parts, "Confirmation dialog")
		if title, ok := state["title"].(string); ok {
			parts = append(parts, title)
		}
		parts = append(parts, "Press Y to confirm or N to reject")

	case "palette":
		parts = append(parts, "Command palette")
		if query, ok := state["query"].(string); ok && query != "" {
			parts = append(parts, fmt.Sprintf("searching for %s", query))
		}
		if count, ok := state["resultCount"].(int); ok {
			parts = append(parts, fmt.Sprintf("%d results", count))
		}
	}

	return strings.Join(parts, ". ")
}

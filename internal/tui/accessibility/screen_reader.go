package accessibility

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ScreenReaderOutput represents different output methods for screen readers
type ScreenReaderOutput int

const (
	// OutputNone disables screen reader output
	OutputNone ScreenReaderOutput = iota
	// OutputFile writes to a file that screen readers can monitor
	OutputFile
	// OutputStderr writes to stderr for screen reader capture
	OutputStderr
	// OutputTTS uses text-to-speech
	OutputTTS
	// OutputBraille sends to Braille display (placeholder)
	OutputBraille
)

// ScreenReader provides screen reader integration
type ScreenReader struct {
	mu sync.Mutex

	enabled    bool
	output     ScreenReaderOutput
	outputFile *os.File
	filePath   string

	// TTS settings
	ttsEngine  string
	ttsRate    int
	ttsPitch   int
	ttsVolume  int
	ttsVoice   string
	ttsProcess *exec.Cmd

	// Queue management
	queue      []Announcement
	processing bool
	stopCh     chan struct{}

	// Callbacks
	onSpeak func(text string)
}

// NewScreenReader creates a new screen reader instance
func NewScreenReader() *ScreenReader {
	return &ScreenReader{
		enabled:   false,
		output:    OutputNone,
		ttsRate:   175,
		ttsPitch:  50,
		ttsVolume: 100,
		queue:     make([]Announcement, 0),
		stopCh:    make(chan struct{}),
	}
}

// SetEnabled enables or disables the screen reader
func (sr *ScreenReader) SetEnabled(enabled bool) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.enabled = enabled
}

// IsEnabled returns whether the screen reader is enabled
func (sr *ScreenReader) IsEnabled() bool {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return sr.enabled
}

// SetOutputMode sets the output mode for the screen reader
func (sr *ScreenReader) SetOutputMode(output ScreenReaderOutput) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	// Close existing file if open
	if sr.outputFile != nil {
		sr.outputFile.Close()
		sr.outputFile = nil
	}

	sr.output = output
	return nil
}

// SetOutputFile sets the file path for file-based output
func (sr *ScreenReader) SetOutputFile(path string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	// Close existing file if open
	if sr.outputFile != nil {
		sr.outputFile.Close()
		sr.outputFile = nil
	}

	sr.filePath = path
	return nil
}

// SetTTSEngine sets the TTS engine to use
func (sr *ScreenReader) SetTTSEngine(engine string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.ttsEngine = engine
}

// SetTTSRate sets the speech rate (words per minute)
func (sr *ScreenReader) SetTTSRate(rate int) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if rate < 50 {
		rate = 50
	}
	if rate > 400 {
		rate = 400
	}
	sr.ttsRate = rate
}

// SetTTSPitch sets the speech pitch (0-100)
func (sr *ScreenReader) SetTTSPitch(pitch int) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if pitch < 0 {
		pitch = 0
	}
	if pitch > 100 {
		pitch = 100
	}
	sr.ttsPitch = pitch
}

// SetTTSVolume sets the speech volume (0-100)
func (sr *ScreenReader) SetTTSVolume(volume int) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	sr.ttsVolume = volume
}

// SetTTSVoice sets the voice to use
func (sr *ScreenReader) SetTTSVoice(voice string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.ttsVoice = voice
}

// SetOnSpeak sets a callback for when text is spoken
func (sr *ScreenReader) SetOnSpeak(fn func(string)) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.onSpeak = fn
}

// Speak queues text for announcement
func (sr *ScreenReader) Speak(ann Announcement) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if !sr.enabled {
		return
	}

	// For assertive/critical, interrupt current speech
	if ann.Priority >= PriorityAssertive {
		sr.interruptLocked()
	}

	sr.queue = append(sr.queue, ann)

	// Start processing if not already
	if !sr.processing {
		sr.processing = true
		go sr.processQueue()
	}
}

// SpeakNow immediately speaks text, interrupting any current speech
func (sr *ScreenReader) SpeakNow(text string) {
	sr.Speak(Announcement{
		Text:      text,
		Priority:  PriorityCritical,
		Timestamp: time.Now(),
	})
}

// Interrupt stops current speech
func (sr *ScreenReader) Interrupt() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.interruptLocked()
}

func (sr *ScreenReader) interruptLocked() {
	// Clear non-critical items from queue
	critical := make([]Announcement, 0)
	for _, ann := range sr.queue {
		if ann.Priority == PriorityCritical {
			critical = append(critical, ann)
		}
	}
	sr.queue = critical

	// Stop current TTS process if running
	if sr.ttsProcess != nil && sr.ttsProcess.Process != nil {
		sr.ttsProcess.Process.Kill()
		sr.ttsProcess = nil
	}
}

// Stop stops all speech processing
func (sr *ScreenReader) Stop() {
	sr.mu.Lock()
	if sr.processing {
		close(sr.stopCh)
	}
	sr.processing = false
	sr.queue = sr.queue[:0]
	if sr.ttsProcess != nil && sr.ttsProcess.Process != nil {
		sr.ttsProcess.Process.Kill()
		sr.ttsProcess = nil
	}
	if sr.outputFile != nil {
		sr.outputFile.Close()
		sr.outputFile = nil
	}
	sr.mu.Unlock()
}

func (sr *ScreenReader) processQueue() {
	for {
		sr.mu.Lock()
		if len(sr.queue) == 0 {
			sr.processing = false
			sr.mu.Unlock()
			return
		}

		// Sort by priority (highest first)
		// For simplicity, just process in order but move critical to front
		var ann Announcement
		criticalIdx := -1
		for i, a := range sr.queue {
			if a.Priority == PriorityCritical {
				criticalIdx = i
				break
			}
		}
		if criticalIdx >= 0 {
			ann = sr.queue[criticalIdx]
			sr.queue = append(sr.queue[:criticalIdx], sr.queue[criticalIdx+1:]...)
		} else {
			ann = sr.queue[0]
			sr.queue = sr.queue[1:]
		}

		output := sr.output
		onSpeak := sr.onSpeak
		sr.mu.Unlock()

		// Output the announcement
		switch output {
		case OutputFile:
			sr.outputToFile(ann)
		case OutputStderr:
			sr.outputToStderr(ann)
		case OutputTTS:
			sr.outputToTTS(ann)
		}

		// Trigger callback
		if onSpeak != nil {
			onSpeak(ann.Text)
		}

		// Small delay between announcements
		time.Sleep(100 * time.Millisecond)
	}
}

func (sr *ScreenReader) outputToFile(ann Announcement) {
	sr.mu.Lock()
	if sr.outputFile == nil && sr.filePath != "" {
		f, err := os.OpenFile(sr.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			sr.outputFile = f
		}
	}
	f := sr.outputFile
	sr.mu.Unlock()

	if f != nil {
		timestamp := ann.Timestamp.Format("15:04:05")
		line := fmt.Sprintf("[%s] [%s] %s\n", timestamp, ann.Priority, ann.Text)
		f.WriteString(line)
		f.Sync()
	}
}

func (sr *ScreenReader) outputToStderr(ann Announcement) {
	// Format: [PRIORITY] TEXT
	prefix := ""
	switch ann.Priority {
	case PriorityCritical:
		prefix = "[ALERT] "
	case PriorityAssertive:
		prefix = "[INFO] "
	default:
		prefix = ""
	}
	fmt.Fprintf(os.Stderr, "%s%s\n", prefix, ann.Text)
}

func (sr *ScreenReader) outputToTTS(ann Announcement) {
	sr.mu.Lock()
	engine := sr.ttsEngine
	rate := sr.ttsRate
	volume := sr.ttsVolume
	voice := sr.ttsVoice
	sr.mu.Unlock()

	text := ann.Text

	// Auto-detect TTS engine if not set
	if engine == "" {
		engine = sr.detectTTSEngine()
	}

	var cmd *exec.Cmd

	switch engine {
	case "espeak":
		args := []string{"-s", fmt.Sprintf("%d", rate)}
		if voice != "" {
			args = append(args, "-v", voice)
		}
		args = append(args, text)
		cmd = exec.Command("espeak", args...)

	case "espeak-ng":
		args := []string{"-s", fmt.Sprintf("%d", rate)}
		if voice != "" {
			args = append(args, "-v", voice)
		}
		args = append(args, text)
		cmd = exec.Command("espeak-ng", args...)

	case "say": // macOS
		args := []string{"-r", fmt.Sprintf("%d", rate)}
		if voice != "" {
			args = append(args, "-v", voice)
		}
		args = append(args, text)
		cmd = exec.Command("say", args...)

	case "festival":
		cmd = exec.Command("festival", "--tts")
		cmd.Stdin = strings.NewReader(text)

	case "spd-say": // Speech Dispatcher
		args := []string{"-r", fmt.Sprintf("%d", (rate-175)/10)} // Normalize rate
		args = append(args, "-V", fmt.Sprintf("%d", volume))
		args = append(args, text)
		cmd = exec.Command("spd-say", args...)

	default:
		// No TTS available, output to stderr instead
		sr.outputToStderr(ann)
		return
	}

	sr.mu.Lock()
	sr.ttsProcess = cmd
	sr.mu.Unlock()

	cmd.Run()

	sr.mu.Lock()
	sr.ttsProcess = nil
	sr.mu.Unlock()
}

func (sr *ScreenReader) detectTTSEngine() string {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("say"); err == nil {
			return "say"
		}
	case "linux":
		// Try common Linux TTS engines in order of preference
		engines := []string{"espeak-ng", "espeak", "spd-say", "festival"}
		for _, engine := range engines {
			if _, err := exec.LookPath(engine); err == nil {
				return engine
			}
		}
	case "windows":
		// Windows uses SAPI through PowerShell
		return "sapi"
	}
	return ""
}

// GetAvailableTTSEngines returns a list of available TTS engines
func GetAvailableTTSEngines() []string {
	var engines []string

	candidates := []string{"say", "espeak-ng", "espeak", "spd-say", "festival"}
	for _, engine := range candidates {
		if _, err := exec.LookPath(engine); err == nil {
			engines = append(engines, engine)
		}
	}

	return engines
}

// GetAvailableVoices returns available voices for the given TTS engine
func GetAvailableVoices(engine string) []string {
	var voices []string

	switch engine {
	case "espeak", "espeak-ng":
		cmd := exec.Command(engine, "--voices")
		output, err := cmd.Output()
		if err == nil {
			lines := strings.Split(string(output), "\n")
			for i, line := range lines {
				if i == 0 {
					continue // Skip header
				}
				fields := strings.Fields(line)
				if len(fields) >= 4 {
					voices = append(voices, fields[4]) // Voice name is 5th column
				}
			}
		}

	case "say": // macOS
		cmd := exec.Command("say", "-v", "?")
		output, err := cmd.Output()
		if err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				fields := strings.Fields(line)
				if len(fields) >= 1 {
					voices = append(voices, fields[0])
				}
			}
		}
	}

	return voices
}

// LiveRegion represents a dynamic content area that should be announced
type LiveRegion struct {
	ID           string
	AriaLive     string // "off", "polite", "assertive"
	AriaAtomic   bool   // Announce entire region or just changes
	AriaRelevant string // "additions", "removals", "text", "all"
	Content      string
	LastContent  string
}

// NewLiveRegion creates a new live region
func NewLiveRegion(id, ariaLive string) *LiveRegion {
	return &LiveRegion{
		ID:           id,
		AriaLive:     ariaLive,
		AriaAtomic:   true,
		AriaRelevant: "all",
	}
}

// Update updates the live region content and returns announcement if changed
func (lr *LiveRegion) Update(content string) (Announcement, bool) {
	if content == lr.LastContent {
		return Announcement{}, false
	}

	lr.LastContent = lr.Content
	lr.Content = content

	if lr.AriaLive == "off" {
		return Announcement{}, false
	}

	priority := PriorityPolite
	if lr.AriaLive == "assertive" {
		priority = PriorityAssertive
	}

	text := content
	if !lr.AriaAtomic && lr.LastContent != "" {
		// Only announce the difference
		text = strings.TrimPrefix(content, lr.LastContent)
		text = strings.TrimSpace(text)
	}

	return Announcement{
		Text:      text,
		Priority:  priority,
		Role:      "log",
		Timestamp: time.Now(),
	}, text != ""
}

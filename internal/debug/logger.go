package debug

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/config"
)

// Logger handles debug logging of API requests and responses
type Logger struct {
	config    config.DebugConfig
	sessionID string
	mu        sync.Mutex
	file      *os.File
	fileSize  int64
}

// NewLogger creates a new debug logger
func NewLogger(cfg config.DebugConfig, sessionID string) (*Logger, error) {
	fmt.Fprintf(os.Stderr, "[DEBUG] NewLogger called: Enabled=%v, Directory=%s, sessionID=%s\n", cfg.Enabled, cfg.Directory, sessionID)
	if !cfg.Enabled {
		return &Logger{config: cfg, sessionID: sessionID}, nil
	}

	// Create debug directory with session ID
	debugDir := filepath.Join(cfg.Directory, sessionID)
	fmt.Fprintf(os.Stderr, "[DEBUG] Creating debug directory: %s\n", debugDir)
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create debug directory: %w", err)
	}

	// Create debug log file
	timestamp := time.Now().Format("20060102-150405")
	fileName := filepath.Join(debugDir, fmt.Sprintf("api-%s.jsonl", timestamp))
	fmt.Fprintf(os.Stderr, "[DEBUG] Creating debug log file: %s\n", fileName)

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create debug log file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Successfully created logger with file: %s\n", fileName)
	return &Logger{
		config:    cfg,
		sessionID: sessionID,
		file:      file,
	}, nil
}

// LogRequest logs an API request
func (l *Logger) LogRequest(ctx context.Context, provider string, request any) error {
	if !l.config.Enabled || !l.config.SaveRequests {
		return nil
	}

	return l.writeEntry(ctx, "request", provider, request)
}

// LogResponse logs an API response
func (l *Logger) LogResponse(ctx context.Context, provider string, response any, duration time.Duration) error {
	if !l.config.Enabled || !l.config.SaveResponses {
		return nil
	}

	return l.writeEntry(ctx, "response", provider, map[string]any{
		"response": response,
		"duration": duration.String(),
	})
}

// LogError logs an API error
func (l *Logger) LogError(ctx context.Context, provider string, err error, request any) error {
	if !l.config.Enabled {
		return nil
	}

	return l.writeEntry(ctx, "error", provider, map[string]any{
		"error":   err.Error(),
		"request": request,
	})
}

func (l *Logger) writeEntry(ctx context.Context, entryType, provider string, data any) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	entry := DebugEntry{
		Timestamp: time.Now().Format(time.RFC3339Nano),
		SessionID: l.sessionID,
		Type:      entryType,
		Provider:  provider,
		Data:      data,
		Context:   extractContext(ctx),
	}

	jsonData, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	// Check file size limit
	if l.config.MaxFileSize > 0 {
		limit := int64(l.config.MaxFileSize) * 1024 * 1024
		if l.fileSize+int64(len(jsonData))+1 > limit {
			// Rotate file
			if err := l.rotateFile(); err != nil {
				return err
			}
		}
	}

	n, err := l.file.Write(append(jsonData, '\n'))
	if err != nil {
		return err
	}
	l.fileSize += int64(n)
	return l.file.Sync()
}

func (l *Logger) rotateFile() error {
	l.file.Close()

	fileName := filepath.Join(l.config.Directory, l.sessionID, fmt.Sprintf("api-%s.jsonl", time.Now().Format("20060102-150405")))

	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	l.file = file
	l.fileSize = 0
	return nil
}

// Close closes the logger
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// DebugEntry represents a single debug log entry
type DebugEntry struct {
	Timestamp string         `json:"timestamp"`
	SessionID string         `json:"session_id"`
	Type      string         `json:"type"` // request, response, error
	Provider  string         `json:"provider"`
	Data      any            `json:"data"`
	Context   map[string]any `json:"context,omitempty"`
}

func extractContext(ctx context.Context) map[string]any {
	result := make(map[string]any)
	if ctx == nil {
		return result
	}

	// Extract common context values
	if v := ctx.Value("request_id"); v != nil {
		result["request_id"] = v
	}
	if v := ctx.Value("user_id"); v != nil {
		result["user_id"] = v
	}
	if v := ctx.Value("task_id"); v != nil {
		result["task_id"] = v
	}
	return result
}

// NewSessionID generates a new session ID
func NewSessionID() string {
	return fmt.Sprintf("%d-%s", time.Now().Unix(), randomString(8))
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}

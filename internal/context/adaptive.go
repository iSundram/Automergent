package context

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
)

// AdaptiveTokenCalculator estimates tokens from characters and learns a
// per-model correction weight from real API usage (EMA gradient descent).
// This mirrors the gemini-cli adaptive token calculator: the char-count
// heuristic is a base, and a learned weight in [0.5, 2.0] corrects it.
type AdaptiveTokenCalculator struct {
	mu sync.RWMutex

	model  string
	weight float64

	// Learning
	learningRate float64
	maxStep      float64
	minWeight    float64
	maxWeight    float64

	// State
	samples     int
	lastGround  float64
	path        string // optional persistence path
}

// NewAdaptiveTokenCalculator creates a calculator with EMA learning.
func NewAdaptiveTokenCalculator(model string) *AdaptiveTokenCalculator {
	c := &AdaptiveTokenCalculator{
		model:        model,
		weight:       1.0,
		learningRate: 0.08,
		maxStep:      0.15,
		minWeight:    0.5,
		maxWeight:    2.0,
	}
	return c
}

// WithPersistence enables loading/saving the learned weight to disk.
func (c *AdaptiveTokenCalculator) WithPersistence(dir string) *AdaptiveTokenCalculator {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.path = filepath.Join(dir, "token_calibration.json")
	c.load()
	return c
}

// Model returns the model name this calculator is bound to.
func (c *AdaptiveTokenCalculator) Model() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

// Estimate estimates tokens for text using the learned weight.
func (c *AdaptiveTokenCalculator) Estimate(text string) int {
	c.mu.RLock()
	weight := c.weight
	c.mu.RUnlock()
	base := float64(len(text)) / 4.0
	return int(math.Round(base * weight))
}

// EstimateMessages estimates tokens for a message slice.
func (c *AdaptiveTokenCalculator) EstimateMessages(messages []ai.Message) int {
	total := 0
	for _, m := range messages {
		total += c.Estimate(m.Plaintext())
	}
	return total
}

// RecordGroundTruth updates the learned weight from real API usage. The ground
// truth is the actual prompt token count observed for a request.
func (c *AdaptiveTokenCalculator) RecordGroundTruth(promptText string, actualTokens int) {
	if actualTokens <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	base := float64(len(promptText)) / 4.0
	if base <= 0 {
		return
	}
	rawTarget := float64(actualTokens) / base

	old := c.weight
	// Clamp the target to avoid a single outlier dominating.
	target := math.Max(old*0.5, math.Min(rawTarget, old*2.0))

	// EMA with bounded step size.
	step := c.learningRate
	if diff := math.Abs(target - old); diff > c.maxStep {
		step = c.maxStep / math.Max(diff, 1e-9)
	}
	newWeight := old*(1-step) + target*step

	c.weight = math.Max(c.minWeight, math.Min(newWeight, c.maxWeight))
	c.samples++
	c.lastGround = float64(actualTokens)

	if c.path != "" {
		_ = c.save()
	}
}

// Weight returns the current learned weight.
func (c *AdaptiveTokenCalculator) Weight() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.weight
}

// Samples returns the number of ground-truth samples recorded.
func (c *AdaptiveTokenCalculator) Samples() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.samples
}

type calibrationFile struct {
	Model     string    `json:"model"`
	Weight    float64   `json:"weight"`
	Samples   int       `json:"samples"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *AdaptiveTokenCalculator) save() error {
	data, _ := json.MarshalIndent(calibrationFile{
		Model:     c.model,
		Weight:    c.weight,
		Samples:   c.samples,
		UpdatedAt: time.Now(),
	}, "", "  ")
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	return atomicWriteFile(c.path, data, 0o600)
}

func (c *AdaptiveTokenCalculator) load() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var cf calibrationFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil
	}
	if cf.Model == c.model && cf.Weight > 0 {
		c.weight = math.Max(c.minWeight, math.Min(cf.Weight, c.maxWeight))
		c.samples = cf.Samples
	}
	return nil
}

// MessageLike abstracts ai.Message for token estimation without an import cycle.
type MessageLike interface {
	Plaintext() string
}

// Reset clears learned state.
func (c *AdaptiveTokenCalculator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.weight = 1.0
	c.samples = 0
	c.lastGround = 0
}
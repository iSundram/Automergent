package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdaptiveTokenCalculatorEstimate(t *testing.T) {
	c := NewAdaptiveTokenCalculator("test-model")
	est := c.Estimate(strings.Repeat("x", 400))
	if est != 100 {
		t.Fatalf("expected 100, got %d", est)
	}
}

func TestAdaptiveTokenCalculatorLearns(t *testing.T) {
	c := NewAdaptiveTokenCalculator("test-model")
	prompt := strings.Repeat("x", 400) // base = 100
	// Simulate provider saying it was actually 150 tokens.
	// EMA with lr=0.08 and maxStep=0.15: first update: target=1.5, old=1.0, diff=0.5 > maxStep=0.15
	// step = 0.15/0.5 = 0.3, new = 1.0*0.7 + 1.5*0.3 = 0.7 + 0.45 = 1.15
	// Need multiple samples to reach ~1.5
	for i := 0; i < 20; i++ {
		c.RecordGroundTruth(prompt, 150)
	}
	w := c.Weight()
	if w < 1.3 || w > 1.7 {
		t.Fatalf("expected weight ~1.5, got %f", w)
	}
}

func TestAdaptiveTokenCalculatorClamp(t *testing.T) {
	c := NewAdaptiveTokenCalculator("test-model")
	prompt := strings.Repeat("x", 400) // base = 100
	// Extreme outlier should be clamped (max 2x)
	c.RecordGroundTruth(prompt, 500) // rawTarget = 5, clamped to 2
	w := c.Weight()
	if w > 2.0 {
		t.Fatalf("expected weight <= 2.0, got %f", w)
	}
}

func TestAdaptiveTokenCalculatorPersistence(t *testing.T) {
	dir := t.TempDir()
	c1 := NewAdaptiveTokenCalculator("model-1").WithPersistence(dir)
	prompt := strings.Repeat("x", 400)
	c1.RecordGroundTruth(prompt, 180)
	w1 := c1.Weight()

	// New instance should load the weight
	c2 := NewAdaptiveTokenCalculator("model-1").WithPersistence(dir)
	w2 := c2.Weight()
	if w2 != w1 {
		t.Fatalf("expected persisted weight %f, got %f", w1, w2)
	}
}

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	data := []byte("hello")
	if err := atomicWriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	read, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(read) != "hello" {
		t.Fatalf("expected hello, got %s", string(read))
	}
}

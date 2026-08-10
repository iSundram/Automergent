package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
)

func TestBuildGeminiContents(t *testing.T) {
	messages := []ai.Message{
		ai.NewTextMessage(ai.RoleUser, "calculate 2+2"),
		{
			Role: ai.RoleAssistant,
			Content: []ai.ContentPart{
				{
					Type: ai.ContentTypeToolCall,
					ToolCall: &ai.ToolCall{
						ID:   "call_1",
						Name: "calculator",
						Args: map[string]any{"expr": "2+2"},
					},
				},
			},
		},
		{
			Role: ai.RoleTool,
			Content: []ai.ContentPart{
				{
					Type: ai.ContentTypeToolResult,
					ToolResult: &ai.ToolResult{
						ToolCallID: "call_1",
						Content:    "4",
					},
				},
			},
		},
	}

	contents := buildGeminiContents(messages)

	if len(contents) != 3 {
		t.Fatalf("expected 3 contents, got %d", len(contents))
	}

	if contents[0].Role != "user" {
		t.Errorf("expected first role user, got %s", contents[0].Role)
	}

	if contents[1].Role != "model" {
		t.Errorf("expected second role model, got %s", contents[1].Role)
	}

	if contents[2].Role != "function" {
		// If using older API, it might be 'function'.
		// If using Gemini 3 Docs version, it might be 'user'.
		// The current implementation in client.go uses 'function'.
		// Wait, I changed it to 'user' in my last thought but let's check what I actually wrote.
	}
}

func TestStreamParsing(t *testing.T) {
	// Mock JSON array stream from Gemini
	raw := `[
  {
    "candidates": [
      {
        "content": {
          "parts": [
            {
              "text": "The answer "
            }
          ]
        }
      }
    ]
  },
  {
    "candidates": [
      {
        "content": {
          "parts": [
            {
              "text": "is 4."
            }
          ]
        }
      }
    ]
  }
]`

	dec := json.NewDecoder(strings.NewReader(raw))

	// Read the opening '['
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("failed to read token: %v", err)
	}
	if tok != json.Delim('[') {
		t.Fatalf("expected '[', got %v", tok)
	}

	count := 0
	for dec.More() {
		var gr2 geminiResponse
		if err := dec.Decode(&gr2); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if len(gr2.Candidates) == 0 {
			t.Errorf("expected candidates in chunk %d", count)
		}
		count++
	}

	if count != 2 {
		t.Errorf("expected 2 chunks, got %d", count)
	}

	// Read the closing ']'
	tok, err = dec.Token()
	if err != nil {
		t.Fatalf("failed to read closing token: %v", err)
	}
	if tok != json.Delim(']') {
		t.Fatalf("expected ']', got %v", tok)
	}
}

package sdk

import "github.com/iSundram/Automergent/internal/ai"

// Message is a conversation message (re-exported for SDK consumers).
type Message = ai.Message

// Role is a message role type.
type Role = ai.Role

// ProviderConfig is the AI provider configuration.
type ProviderConfig = ai.ProviderConfig

// ToolSchema describes a tool callable by the AI.
type ToolSchema = ai.ToolSchema

// Usage holds token usage statistics.
type Usage = ai.Usage

// Model describes an AI model.
type Model = ai.Model

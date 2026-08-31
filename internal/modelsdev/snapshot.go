package modelsdev

import "github.com/iSundram/Automergent/internal/ai"

// snapshotModels is the embedded fallback catalog for the google provider,
// generated from https://models.dev/api.json (tool-call capable models
// only). It is the last resort when neither the disk cache nor the network
// is available.
var snapshotModels = []ai.Model{
	{ID: "deep-research-max-preview-04-2026", Name: "Deep Research Max Preview (Apr-21-2026)", ContextLimit: 131072, OutputLimit: 65536, InputPrice: 2.0000, OutputPrice: 12.0000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "deep-research-preview-04-2026", Name: "Deep Research Preview (Apr-21-2026)", ContextLimit: 131072, OutputLimit: 65536, InputPrice: 2.0000, OutputPrice: 12.0000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-2.5-computer-use-preview-10-2025", Name: "Gemini 2.5 Computer Use Preview 10-2025", ContextLimit: 131072, OutputLimit: 65536, InputPrice: 1.2500, OutputPrice: 10.0000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 0.3000, OutputPrice: 2.5000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash-Lite", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 0.1000, OutputPrice: 0.4000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 1.2500, OutputPrice: 10.0000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-3-flash-preview", Name: "Gemini 3 Flash Preview", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 0.5000, OutputPrice: 3.0000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-3.1-flash-lite", Name: "Gemini 3.1 Flash Lite", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 0.2500, OutputPrice: 1.5000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-3.1-flash-lite-image", Name: "Nano Banana 2 Lite", ContextLimit: 65536, OutputLimit: 65536, InputPrice: 0.2500, OutputPrice: 30.0000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-3.1-flash-lite-preview", Name: "Gemini 3.1 Flash Lite Preview", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 0.2500, OutputPrice: 1.5000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-3.1-flash-live-preview", Name: "Gemini 3.1 Flash Live Preview", ContextLimit: 131072, OutputLimit: 65536, InputPrice: 0.7500, OutputPrice: 4.5000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-3.1-pro-preview", Name: "Gemini 3.1 Pro Preview", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 2.0000, OutputPrice: 12.0000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-3.1-pro-preview-customtools", Name: "Gemini 3.1 Pro Preview Custom Tools", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 2.0000, OutputPrice: 12.0000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-3.5-flash", Name: "Gemini 3.5 Flash", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 1.5000, OutputPrice: 9.0000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemini-3.5-flash-lite", Name: "Gemini 3.5 Flash Lite", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 0.3000, OutputPrice: 2.5000, Reasoning: true, Attachment: true, Knowledge: "2026-03"},
	{ID: "gemini-3.6-flash", Name: "Gemini 3.6 Flash", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 0.7500, OutputPrice: 3.7500, Reasoning: true, Attachment: true, Knowledge: "2026-03"},
	{ID: "gemini-3.7-flash", Name: "Gemini 3.7 Flash", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 0.7500, OutputPrice: 3.7500, Reasoning: true, Attachment: true, Knowledge: "2026-03"},
	{ID: "gemini-flash-latest", Name: "Gemini Flash Latest", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 0.7500, OutputPrice: 3.7500, Reasoning: true, Attachment: true, Knowledge: "2026-03"},
	{ID: "gemini-flash-lite-latest", Name: "Gemini Flash-Lite Latest", ContextLimit: 1048576, OutputLimit: 65536, InputPrice: 0.3000, OutputPrice: 2.5000, Reasoning: true, Attachment: true, Knowledge: "2026-03"},
	{ID: "gemini-robotics-er-1.6-preview", Name: "Gemini Robotics-ER 1.6 Preview", ContextLimit: 131072, OutputLimit: 65536, InputPrice: 1.0000, OutputPrice: 5.0000, Reasoning: true, Attachment: true, Knowledge: "2025-01"},
	{ID: "gemma-4-26b-a4b-it", Name: "Gemma 4 26B A4B IT", ContextLimit: 262144, OutputLimit: 32768, InputPrice: 0.0000, OutputPrice: 0.0000, Reasoning: true, Attachment: true, Knowledge: ""},
	{ID: "gemma-4-31b-it", Name: "Gemma 4 31B IT", ContextLimit: 262144, OutputLimit: 32768, InputPrice: 0.0000, OutputPrice: 0.0000, Reasoning: true, Attachment: true, Knowledge: ""},
}

package ai

// StaticResponse is a non-streaming CompletionResponse backed by pre-built data.
type StaticResponse struct {
	chunks     chan Chunk
	toolCalls  []ToolCall
	stopReason StopReason
	usage      Usage
	metadata   map[string]any
}

func NewStaticResponse(text, thought string, toolCalls []ToolCall, stop StopReason, usage Usage) *StaticResponse {
	ch := make(chan Chunk, 3)
	if thought != "" {
		ch <- Chunk{Thought: thought}
	}
	if text != "" {
		ch <- Chunk{Text: text}
	}
	ch <- Chunk{Done: true}
	close(ch)
	return &StaticResponse{
		chunks:     ch,
		toolCalls:  toolCalls,
		stopReason: stop,
		usage:      usage,
	}
}

func (r *StaticResponse) Stream() <-chan Chunk         { return r.chunks }
func (r *StaticResponse) ToolCalls() []ToolCall        { return r.toolCalls }
func (r *StaticResponse) StopReason() StopReason       { return r.stopReason }
func (r *StaticResponse) Usage() Usage                 { return r.usage }
func (r *StaticResponse) GetMetadata() map[string]any  { return r.metadata }
func (r *StaticResponse) SetMetadata(m map[string]any) { r.metadata = m }
func (r *StaticResponse) SetStopReason(s StopReason)   { r.stopReason = s }

// ChannelResponse streams chunks from an externally supplied channel.
type ChannelResponse struct {
	ch         <-chan Chunk
	toolCalls  []ToolCall
	stopReason StopReason
	usage      Usage
	metadata   map[string]any
}

func NewChannelResponse(ch <-chan Chunk, stop StopReason, usage Usage) *ChannelResponse {
	return &ChannelResponse{ch: ch, stopReason: stop, usage: usage}
}

func (r *ChannelResponse) Stream() <-chan Chunk         { return r.ch }
func (r *ChannelResponse) ToolCalls() []ToolCall        { return r.toolCalls }
func (r *ChannelResponse) StopReason() StopReason       { return r.stopReason }
func (r *ChannelResponse) Usage() Usage                 { return r.usage }
func (r *ChannelResponse) GetMetadata() map[string]any  { return r.metadata }
func (r *ChannelResponse) SetMetadata(m map[string]any) { r.metadata = m }
func (r *ChannelResponse) SetStopReason(s StopReason)   { r.stopReason = s }
func (r *ChannelResponse) SetToolCalls(t []ToolCall)    { r.toolCalls = t }
func (r *ChannelResponse) SetUsage(u Usage)             { r.usage = u }

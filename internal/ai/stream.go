package ai

// staticResponse is a non-streaming CompletionResponse backed by pre-built data.
type staticResponse struct {
	chunks     chan Chunk
	toolCalls  []ToolCall
	stopReason StopReason
	usage      Usage
	metadata   map[string]any
}

func NewStaticResponse(text, thought string, toolCalls []ToolCall, stop StopReason, usage Usage) *staticResponse {
	ch := make(chan Chunk, 3)
	if thought != "" {
		ch <- Chunk{Thought: thought}
	}
	if text != "" {
		ch <- Chunk{Text: text}
	}
	ch <- Chunk{Done: true}
	close(ch)
	return &staticResponse{
		chunks:     ch,
		toolCalls:  toolCalls,
		stopReason: stop,
		usage:      usage,
	}
}

func (r *staticResponse) Stream() <-chan Chunk         { return r.chunks }
func (r *staticResponse) ToolCalls() []ToolCall        { return r.toolCalls }
func (r *staticResponse) StopReason() StopReason       { return r.stopReason }
func (r *staticResponse) Usage() Usage                 { return r.usage }
func (r *staticResponse) GetMetadata() map[string]any  { return r.metadata }
func (r *staticResponse) SetMetadata(m map[string]any) { r.metadata = m }
func (r *staticResponse) SetStopReason(s StopReason)   { r.stopReason = s }

// channelResponse streams chunks from an externally supplied channel.
type channelResponse struct {
	ch         <-chan Chunk
	toolCalls  []ToolCall
	stopReason StopReason
	usage      Usage
	metadata   map[string]any
}

func NewChannelResponse(ch <-chan Chunk, stop StopReason, usage Usage) *channelResponse {
	return &channelResponse{ch: ch, stopReason: stop, usage: usage}
}

func (r *channelResponse) Stream() <-chan Chunk         { return r.ch }
func (r *channelResponse) ToolCalls() []ToolCall        { return r.toolCalls }
func (r *channelResponse) StopReason() StopReason       { return r.stopReason }
func (r *channelResponse) Usage() Usage                 { return r.usage }
func (r *channelResponse) GetMetadata() map[string]any  { return r.metadata }
func (r *channelResponse) SetMetadata(m map[string]any) { r.metadata = m }
func (r *channelResponse) SetStopReason(s StopReason)   { r.stopReason = s }

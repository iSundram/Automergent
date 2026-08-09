package ai

// staticResponse is a non-streaming CompletionResponse backed by pre-built data.
type staticResponse struct {
	chunks     chan Chunk
	toolCalls  []ToolCall
	stopReason StopReason
	usage      Usage
}

func NewStaticResponse(text string, toolCalls []ToolCall, stop StopReason, usage Usage) CompletionResponse {
	ch := make(chan Chunk, 2)
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

func (r *staticResponse) Stream() <-chan Chunk   { return r.chunks }
func (r *staticResponse) ToolCalls() []ToolCall  { return r.toolCalls }
func (r *staticResponse) StopReason() StopReason { return r.stopReason }
func (r *staticResponse) Usage() Usage           { return r.usage }

// channelResponse streams chunks from an externally supplied channel.
type channelResponse struct {
	ch         <-chan Chunk
	toolCalls  []ToolCall
	stopReason StopReason
	usage      Usage
}

func NewChannelResponse(ch <-chan Chunk, stop StopReason, usage Usage) CompletionResponse {
	return &channelResponse{ch: ch, stopReason: stop, usage: usage}
}

func (r *channelResponse) Stream() <-chan Chunk   { return r.ch }
func (r *channelResponse) ToolCalls() []ToolCall  { return r.toolCalls }
func (r *channelResponse) StopReason() StopReason { return r.stopReason }
func (r *channelResponse) Usage() Usage           { return r.usage }

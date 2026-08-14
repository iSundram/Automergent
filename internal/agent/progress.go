package agent

// LongTaskStatus describes structured long-task progress updates.
type LongTaskStatus struct {
	TaskID      string  `json:"task_id,omitempty"`
	Phase       string  `json:"phase,omitempty"`
	ProgressPct float64 `json:"progress_pct,omitempty"`
	ETASec      int64   `json:"eta_sec,omitempty"`
	Log         string  `json:"log,omitempty"`
	Message     string  `json:"message,omitempty"`
}

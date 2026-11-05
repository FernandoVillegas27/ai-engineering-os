package controlplane

type RunControls struct {
	Consistent       bool     `json:"consistent"`
	CanPause         bool     `json:"can_pause"`
	CanResume        bool     `json:"can_resume"`
	CanCancel        bool     `json:"can_cancel"`
	CanRetry         bool     `json:"can_retry"`
	CanRecover       bool     `json:"can_recover"`
	RecoveryTo       string   `json:"recovery_to,omitempty"`
	RecoveryFrom     string   `json:"recovery_from,omitempty"`
	PauseBoundary    string   `json:"pause_boundary,omitempty"`
	ResumeFrom       string   `json:"resume_from,omitempty"`
	RetryableTaskIDs []string `json:"retryable_task_ids,omitempty"`
	Reason           string   `json:"reason,omitempty"`
}

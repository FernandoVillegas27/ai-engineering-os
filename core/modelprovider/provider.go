package modelprovider

import (
	"context"

	"github.com/FernandoVillegas27/ai-engineering-os/core/orchestration"
)

type CapabilityOption struct {
	ID          string
	Description string
	Aliases     []string
}

type ClassificationRequest struct {
	Task                string
	ProjectContext      string
	AllowedIntents      []orchestration.Intent
	AllowedCapabilities []CapabilityOption
}

type ClassificationResponse struct {
	Intent                orchestration.Intent
	Capabilities          []string
	CanClassify           bool
	ClarificationQuestion string
	Rationale             string
}

type Provider interface {
	ID() string
	Classify(ctx context.Context, request ClassificationRequest) (ClassificationResponse, error)
}

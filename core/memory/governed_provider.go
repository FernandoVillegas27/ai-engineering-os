package memory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type GovernedProjectMemory struct {
	Store  *GovernanceStore
	Mirror ProjectMemory
	Now    func() time.Time
}

func (g GovernedProjectMemory) ID() string {
	if g.Mirror != nil {
		return "governed+" + g.Mirror.ID()
	}
	return "governed-local"
}

func (g GovernedProjectMemory) Status(ctx context.Context) (Status, error) {
	if g.Store == nil {
		return Status{Provider: g.ID(), Available: false, Healthy: false, Details: "memory governance store is not configured"}, nil
	}
	details := "local governed memory ready"
	if g.Mirror != nil {
		status, err := g.Mirror.Status(ctx)
		if err != nil {
			details += "; mirror status error: " + err.Error()
		} else if !status.Available || !status.Healthy {
			mirrorDetail := strings.TrimSpace(status.Details)
			if mirrorDetail == "" {
				mirrorDetail = "mirror unavailable"
			}
			details += "; mirror degraded: " + mirrorDetail
		} else {
			details += "; mirror=" + g.Mirror.ID() + " healthy"
		}
	}
	return Status{
		Provider:  g.ID(),
		Available: true,
		Healthy:   true,
		Details:   details,
	}, nil
}

func (g GovernedProjectMemory) Save(ctx context.Context, request SaveRequest) (SaveResult, error) {
	if g.Store == nil {
		return SaveResult{}, fmt.Errorf("memory governance store is required")
	}
	now := time.Now().UTC()
	if g.Now != nil {
		now = g.Now().UTC()
	}
	record, err := g.Store.Save(GovernedSaveRequest{
		SaveRequest: request,
		Provider:    mirrorID(g.Mirror),
		Provenance:  request.Provenance,
		Retention:   request.Retention,
	}, now)
	if err != nil {
		return SaveResult{}, err
	}

	summary := "governed record " + record.ID
	if g.Mirror != nil {
		mirrored := request
		mirrored.Title = record.Title
		mirrored.Content = record.Content
		if result, mirrorErr := g.Mirror.Save(ctx, mirrored); mirrorErr != nil {
			summary += "; mirror warning: " + mirrorErr.Error()
		} else if strings.TrimSpace(result.Summary) != "" {
			summary += "; mirror: " + strings.TrimSpace(result.Summary)
		}
	}
	return SaveResult{
		Provider: g.ID(),
		Project:  record.Project,
		Saved:    true,
		Summary:  summary,
	}, nil
}

func (g GovernedProjectMemory) Recall(_ context.Context, request RecallRequest) (RecallResult, error) {
	if g.Store == nil {
		return RecallResult{}, fmt.Errorf("memory governance store is required")
	}
	now := time.Now().UTC()
	if g.Now != nil {
		now = g.Now().UTC()
	}
	values, err := g.Store.Recall(request.Project, request.Query, request.Limit, now)
	if err != nil {
		return RecallResult{}, err
	}
	var b strings.Builder
	for _, record := range values {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "[%s] %s\n%s", record.ID, record.Title, record.Content)
		if record.Provenance.RunID != "" || record.Provenance.ArtifactID != "" || record.Provenance.Source != "" {
			b.WriteString("\nprovenance:")
			if record.Provenance.RunID != "" {
				b.WriteString(" run=" + record.Provenance.RunID)
			}
			if record.Provenance.ArtifactID != "" {
				b.WriteString(" artifact=" + record.Provenance.ArtifactID)
			}
			if record.Provenance.Source != "" {
				b.WriteString(" source=" + record.Provenance.Source)
			}
		}
	}
	return RecallResult{
		Provider: g.ID(),
		Project:  strings.TrimSpace(request.Project),
		Query:    strings.TrimSpace(request.Query),
		Context:  strings.TrimSpace(b.String()),
	}, nil
}

func mirrorID(provider ProjectMemory) string {
	if provider == nil {
		return ""
	}
	return provider.ID()
}

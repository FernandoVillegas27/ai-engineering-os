package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Status struct {
	Provider  string
	Available bool
	Healthy   bool
	Version   string
	Details   string
}

type Provider interface {
	ID() string
	Status(ctx context.Context) (Status, error)
}

type Type string

const (
	TypeDecision     Type = "decision"
	TypeArchitecture Type = "architecture"
	TypeBugfix       Type = "bugfix"
	TypePattern      Type = "pattern"
	TypeConfig       Type = "config"
	TypeDiscovery    Type = "discovery"
	TypeLearning     Type = "learning"
)

type SaveRequest struct {
	Project  string
	Title    string
	Content  string
	Type     Type
	TopicKey string
}

type SaveResult struct {
	Provider string
	Project  string
	Saved    bool
	Summary  string
}

type RecallRequest struct {
	Project string
	Query   string
	Limit   int
}

type RecallResult struct {
	Provider string
	Project  string
	Query    string
	Context  string
}

type ProjectMemory interface {
	Provider
	Save(ctx context.Context, request SaveRequest) (SaveResult, error)
	Recall(ctx context.Context, request RecallRequest) (RecallResult, error)
}

func ValidateSaveRequest(request SaveRequest) error {
	if err := validateProject(request.Project); err != nil {
		return err
	}
	if strings.TrimSpace(request.Title) == "" {
		return errors.New("memory title is required")
	}
	if strings.TrimSpace(request.Content) == "" {
		return errors.New("memory content is required")
	}
	if len(request.Title) > 200 {
		return errors.New("memory title exceeds 200 characters")
	}
	if len(request.Content) > 16*1024 {
		return errors.New("memory content exceeds 16 KiB")
	}
	if !validType(request.Type) {
		return fmt.Errorf("unsupported memory type: %q", request.Type)
	}
	if strings.ContainsAny(request.TopicKey, "\r\n") {
		return errors.New("memory topic key must be one line")
	}
	return nil
}

func ValidateRecallRequest(request RecallRequest) error {
	if err := validateProject(request.Project); err != nil {
		return err
	}
	if strings.TrimSpace(request.Query) == "" {
		return errors.New("memory recall query is required")
	}
	if len(request.Query) > 1000 {
		return errors.New("memory recall query exceeds 1000 characters")
	}
	if request.Limit < 0 || request.Limit > 20 {
		return errors.New("memory recall limit must be between 0 and 20")
	}
	return nil
}

func validateProject(project string) error {
	project = strings.TrimSpace(project)
	if project == "" {
		return errors.New("memory project namespace is required")
	}
	if len(project) > 200 {
		return errors.New("memory project namespace exceeds 200 characters")
	}
	if strings.ContainsAny(project, "\r\n") {
		return errors.New("memory project namespace must be one line")
	}
	return nil
}

func validType(value Type) bool {
	switch value {
	case TypeDecision, TypeArchitecture, TypeBugfix, TypePattern, TypeConfig, TypeDiscovery, TypeLearning:
		return true
	default:
		return false
	}
}

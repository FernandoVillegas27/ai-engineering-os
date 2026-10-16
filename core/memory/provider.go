package memory

import "context"

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

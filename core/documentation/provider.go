package documentation

import "context"

type Status struct {
	Provider  string
	Available bool
	Ready     bool
	Version   string
	Details   string
}

type Provider interface {
	ID() string
	Status(ctx context.Context) (Status, error)
}

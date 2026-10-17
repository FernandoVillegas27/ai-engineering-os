package projection

import "context"

type Status struct {
	Provider  string
	Available bool
	Healthy   bool
	Version   string
	Details   string
}

type Plan struct {
	Provider string
	Target   string
	Mode     string
	Command  string
	Args     []string
	Mutates  bool
}

type Provider interface {
	ID() string
	Status(ctx context.Context) (Status, error)
	PreviewInstall(target string) (Plan, error)
	PreviewSync(target string) (Plan, error)
}

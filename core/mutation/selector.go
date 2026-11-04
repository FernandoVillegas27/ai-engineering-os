package mutation

import (
	"errors"
	"sort"

	"github.com/FernandoVillegas27/ai-engineering-os/core/execution"
)

// SelectSourceMutatingUnits returns only work units whose registry-derived
// metadata explicitly allows source-code writes. Effective permissions remain
// read-only; StageRuntime owns isolated workspace mutation.
func SelectSourceMutatingUnits(units []execution.WorkUnit) ([]execution.WorkUnit, error) {
	var out []execution.WorkUnit
	for _, unit := range units {
		if unit.Mutation.SourceCode {
			out = append(out, unit)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("coordinated mutation selected no source-code-write work units")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

package mutation

import (
	"fmt"

	"github.com/FernandoVillegas27/ai-engineering-os/core/filekind"
)

func ValidateFrozenTestsOnly(values []FrozenProposal) error {
	for _, value := range values {
		if err := filekind.ValidateTestsOnly(value.Declared); err != nil {
			return fmt.Errorf("tests-only proposal %s (%s): %w", value.TaskID, value.AgentID, err)
		}
	}
	return nil
}

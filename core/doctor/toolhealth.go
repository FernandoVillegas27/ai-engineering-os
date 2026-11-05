package doctor

import (
	"fmt"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/toolhealth"
)

func CheckExternalTool(id, label string, status toolhealth.Status) Check {
	id = strings.TrimSpace(id)
	label = strings.TrimSpace(label)
	if id == "" {
		id = status.Provider
	}
	if label == "" {
		label = status.Provider
	}
	if !status.Available {
		hint := strings.TrimSpace(status.Details)
		if hint == "" {
			hint = "Provider is unavailable."
		}
		return Check{
			ID: id, Label: label, Level: LevelWarn,
			Message: "Proveedor externo no disponible.",
			Hint: healthHint(status, hint),
		}
	}
	if !status.Healthy {
		hint := strings.TrimSpace(status.Details)
		if hint == "" {
			hint = "Provider health probe failed."
		}
		return Check{
			ID: id, Label: label, Level: LevelWarn,
			Message: "Proveedor externo requiere atención.",
			Hint: healthHint(status, hint),
		}
	}
	message := "Proveedor externo saludable."
	if version := strings.TrimSpace(status.Version); version != "" {
		message = version
	}
	return Check{
		ID: id, Label: label, Level: LevelOK, Message: message,
	}
}

func healthHint(status toolhealth.Status, fallback string) string {
	failure := status.Failure
	if failure.Kind == toolhealth.FailureNone {
		return fallback
	}
	retry := "no"
	if failure.Retryable {
		retry = "sí"
	}
	detail := strings.TrimSpace(failure.Detail)
	if detail == "" {
		detail = fallback
	}
	return fmt.Sprintf("kind=%s retryable=%s · %s", failure.Kind, retry, detail)
}

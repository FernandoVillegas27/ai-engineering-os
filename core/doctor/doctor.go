package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/FernandoVillegas27/ai-engineering-os/core/registry"
)

type Level string

const (
	LevelOK    Level = "ok"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

type Check struct {
	ID      string
	Label   string
	Level   Level
	Message string
	Hint    string
}

type Report struct {
	Checks []Check
}

func (r Report) HasErrors() bool {
	for _, check := range r.Checks {
		if check.Level == LevelError {
			return true
		}
	}
	return false
}

func CheckWorkspace(root string) Check {
	info, err := os.Stat(root)
	if err != nil {
		return Check{
			ID:      "workspace",
			Label:   "Workspace",
			Level:   LevelError,
			Message: "No se pudo acceder al workspace.",
			Hint:    err.Error(),
		}
	}
	if !info.IsDir() {
		return Check{
			ID:      "workspace",
			Label:   "Workspace",
			Level:   LevelError,
			Message: "La ruta del proyecto no es un directorio.",
		}
	}

	return Check{
		ID:      "workspace",
		Label:   "Workspace",
		Level:   LevelOK,
		Message: "Workspace accesible.",
	}
}

func CheckGit(root string) Check {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return Check{
			ID:      "git",
			Label:   "Git",
			Level:   LevelError,
			Message: "Git no está disponible en PATH.",
			Hint:    "Instalá Git o corregí PATH.",
		}
	}

	cmd := exec.Command(gitPath, "-C", root, "rev-parse", "--is-inside-work-tree")
	output, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "true" {
		return Check{
			ID:      "git",
			Label:   "Git",
			Level:   LevelWarn,
			Message: "El workspace no parece ser un repositorio Git.",
			Hint:    "Inicializá Git si este proyecto debe estar versionado.",
		}
	}

	return Check{
		ID:      "git",
		Label:   "Git",
		Level:   LevelOK,
		Message: "Repositorio Git detectado.",
	}
}

func CheckSkillRegistry(path string) Check {
	if strings.TrimSpace(path) == "" {
		return Check{
			ID:      "skill-registry",
			Label:   "Skill Registry",
			Level:   LevelError,
			Message: "No se resolvió la ruta del Skill Registry.",
			Hint:    "Usá --registry o configurá AI_ENGINEERING_OS_HOME.",
		}
	}

	loaded, err := registry.LoadSkills(path)
	if err != nil {
		return Check{
			ID:      "skill-registry",
			Label:   "Skill Registry",
			Level:   LevelError,
			Message: "El Skill Registry no pudo cargarse.",
			Hint:    err.Error(),
		}
	}

	return Check{
		ID:      "skill-registry",
		Label:   "Skill Registry",
		Level:   LevelOK,
		Message: fmt.Sprintf("Registry válido (%d skills).", len(loaded.Skills)),
	}
}

func CheckProjectContext(root string) Check {
	path := filepath.Join(root, ".ai", "project.yaml")
	if _, err := os.Stat(path); err == nil {
		return Check{
			ID:      "project-context",
			Label:   "Project Context",
			Level:   LevelOK,
			Message: ".ai/project.yaml presente.",
		}
	} else if !os.IsNotExist(err) {
		return Check{
			ID:      "project-context",
			Label:   "Project Context",
			Level:   LevelWarn,
			Message: "No se pudo verificar .ai/project.yaml.",
			Hint:    err.Error(),
		}
	}

	return Check{
		ID:      "project-context",
		Label:   "Project Context",
		Level:   LevelWarn,
		Message: ".ai/project.yaml todavía no existe.",
		Hint:    "Ejecutá ai inspect <proyecto> --write cuando quieras persistir el contexto.",
	}
}

type engramProjectConfig struct {
	ProjectName string `json:"project_name"`
}

func CheckEngramProject(root string) Check {
	path := filepath.Join(root, ".engram", "config.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Check{
			ID:      "engram-project",
			Label:   "Engram Project",
			Level:   LevelWarn,
			Message: "El proyecto de Engram no está fijado.",
			Hint:    "Ejecutá engram init <nombre-del-proyecto> desde el repo.",
		}
	}
	if err != nil {
		return Check{
			ID:      "engram-project",
			Label:   "Engram Project",
			Level:   LevelWarn,
			Message: "No se pudo leer .engram/config.json.",
			Hint:    err.Error(),
		}
	}

	var config engramProjectConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return Check{
			ID:      "engram-project",
			Label:   "Engram Project",
			Level:   LevelWarn,
			Message: ".engram/config.json no es válido.",
			Hint:    err.Error(),
		}
	}
	if strings.TrimSpace(config.ProjectName) == "" {
		return Check{
			ID:      "engram-project",
			Label:   "Engram Project",
			Level:   LevelWarn,
			Message: "Engram no tiene project_name configurado.",
			Hint:    "Ejecutá engram init <nombre-del-proyecto> --force.",
		}
	}

	return Check{
		ID:      "engram-project",
		Label:   "Engram Project",
		Level:   LevelOK,
		Message: fmt.Sprintf("Scope fijado en %s.", config.ProjectName),
	}
}

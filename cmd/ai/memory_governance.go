package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/memory"
	engramintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/engram"
)

type memoryGovernanceOptions struct {
	project      string
	governanceID string
	title        string
	content      string
	memoryType   memory.Type
	hard         bool
	yes          bool
	json         bool
	olderThan    time.Duration
}

func runMemoryGovernance(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: ai memory list|review|delete|supersede|prune ...")
	}
	switch args[0] {
	case "list":
		return runMemoryList(args[1:])
	case "review":
		return runMemoryReview(args[1:])
	case "delete":
		return runMemoryDelete(args[1:])
	case "supersede":
		return runMemorySupersede(args[1:])
	case "prune":
		return runMemoryPrune(args[1:])
	default:
		return fmt.Errorf("subcomando de memoria desconocido: %s", args[0])
	}
}

func openGovernedEngram() (memory.GovernedProvider, error) {
	store, err := openMemoryGovernanceStore()
	if err != nil {
		return memory.GovernedProvider{}, err
	}
	return memory.GovernedProvider{
		Inner: engramintegration.New(),
		Store: store,
	}, nil
}

func runMemoryList(args []string) error {
	options, err := parseMemoryGovernanceArgs("list", args)
	if err != nil {
		return err
	}
	store, err := openMemoryGovernanceStore()
	if err != nil {
		return err
	}
	records, err := store.Records(options.project)
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(records)
	}
	fmt.Printf("Project memory: %s\n", options.project)
	if len(records) == 0 {
		fmt.Println("No governed memories.")
		return nil
	}
	for _, record := range records {
		reviewed := "no"
		if record.ReviewedAt != nil {
			reviewed = "yes"
		}
		fmt.Printf(
			"- %s [%s] type=%s provider=%s memory=%s reviewed=%s\n",
			record.GovernanceID,
			record.State,
			record.Type,
			record.Provider,
			record.MemoryID,
			reviewed,
		)
		fmt.Printf("  title: %s\n", record.Title)
		if record.TopicKey != "" {
			fmt.Printf("  topic: %s\n", record.TopicKey)
		}
		if record.Provenance.RunID != "" {
			fmt.Printf(
				"  provenance: run=%s task=%s agent=%s source=%s\n",
				record.Provenance.RunID,
				record.Provenance.TaskID,
				record.Provenance.AgentID,
				record.Provenance.Source,
			)
		}
	}
	return nil
}

func runMemoryReview(args []string) error {
	options, err := parseMemoryGovernanceArgs("review", args)
	if err != nil {
		return err
	}
	governed, err := openGovernedEngram()
	if err != nil {
		return err
	}
	record, err := governed.Review(context.Background(), options.project, options.governanceID)
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(record)
	}
	fmt.Printf("Memory reviewed: %s\n", record.GovernanceID)
	fmt.Printf("Project: %s\n", record.Project)
	fmt.Printf("State: %s\n", record.State)
	return nil
}

func runMemoryDelete(args []string) error {
	options, err := parseMemoryGovernanceArgs("delete", args)
	if err != nil {
		return err
	}
	if options.hard && !options.yes {
		return fmt.Errorf("hard delete requiere --yes")
	}
	governed, err := openGovernedEngram()
	if err != nil {
		return err
	}
	record, err := governed.Delete(
		context.Background(),
		options.project,
		options.governanceID,
		options.hard,
	)
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(record)
	}
	fmt.Printf("Memory deleted: %s\n", record.GovernanceID)
	fmt.Printf("Project: %s\n", record.Project)
	fmt.Printf("Hard delete: %t\n", record.HardDeleted)
	return nil
}

func runMemorySupersede(args []string) error {
	options, err := parseMemoryGovernanceArgs("supersede", args)
	if err != nil {
		return err
	}
	governed, err := openGovernedEngram()
	if err != nil {
		return err
	}
	previous, err := governed.Store.Record(options.project, options.governanceID)
	if err != nil {
		return err
	}
	if previous.State != memory.GovernanceActive {
		return fmt.Errorf("solo una memoria activa puede ser reemplazada, state=%s", previous.State)
	}
	if strings.TrimSpace(previous.TopicKey) == "" {
		return fmt.Errorf("la memoria no tiene topic_key estable y no puede reemplazarse de forma segura")
	}
	memoryType := options.memoryType
	if memoryType == "" {
		memoryType = previous.Type
	}
	result, err := governed.Save(context.Background(), memory.SaveRequest{
		Project:  options.project,
		Title:    options.title,
		Content:  options.content,
		Type:     memoryType,
		TopicKey: previous.TopicKey,
		Provenance: memory.Provenance{
			Source: "manual-supersede",
		},
	})
	if err != nil {
		return err
	}
	record, err := governed.Store.Record(options.project, result.GovernanceID)
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(record)
	}
	fmt.Printf("Memory superseded: %s -> %s\n", previous.GovernanceID, record.GovernanceID)
	fmt.Printf("Topic: %s\n", record.TopicKey)
	return nil
}

func runMemoryPrune(args []string) error {
	options, err := parseMemoryGovernanceArgs("prune", args)
	if err != nil {
		return err
	}
	if !options.yes {
		return fmt.Errorf("prune requiere --yes porque puede borrar múltiples memorias")
	}
	governed, err := openGovernedEngram()
	if err != nil {
		return err
	}
	records, err := governed.Prune(
		context.Background(),
		options.project,
		options.olderThan,
		options.hard,
	)
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(records)
	}
	fmt.Printf("Memory prune: %d record(s) deleted\n", len(records))
	fmt.Printf("Project: %s\n", options.project)
	fmt.Printf("Older than: %s\n", options.olderThan)
	fmt.Printf("Hard delete: %t\n", options.hard)
	return nil
}

func parseMemoryGovernanceArgs(command string, args []string) (memoryGovernanceOptions, error) {
	var options memoryGovernanceOptions
	options.memoryType = ""
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if i+1 >= len(args) {
				return options, fmt.Errorf("--project requiere un id")
			}
			i++
			options.project = strings.TrimSpace(args[i])
		case "--title":
			if i+1 >= len(args) {
				return options, fmt.Errorf("--title requiere texto")
			}
			i++
			options.title = strings.TrimSpace(args[i])
		case "--content":
			if i+1 >= len(args) {
				return options, fmt.Errorf("--content requiere texto")
			}
			i++
			options.content = strings.TrimSpace(args[i])
		case "--type":
			if i+1 >= len(args) {
				return options, fmt.Errorf("--type requiere un tipo")
			}
			i++
			value, err := parseMemoryType(args[i])
			if err != nil {
				return options, err
			}
			options.memoryType = value
		case "--older-than":
			if i+1 >= len(args) {
				return options, fmt.Errorf("--older-than requiere una duración, por ejemplo 90d")
			}
			i++
			value, err := parseRetentionDuration(args[i])
			if err != nil {
				return options, err
			}
			options.olderThan = value
		case "--hard":
			options.hard = true
		case "--yes":
			options.yes = true
		case "--json":
			options.json = true
		case "-h", "--help":
			return options, memoryGovernanceUsage(command)
		default:
			if strings.HasPrefix(args[i], "-") {
				return options, fmt.Errorf("opción desconocida: %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if options.project == "" {
		return options, fmt.Errorf("--project es obligatorio")
	}
	switch command {
	case "list":
		if len(positional) != 0 {
			return options, memoryGovernanceUsage(command)
		}
	case "review", "delete", "supersede":
		if len(positional) != 1 {
			return options, memoryGovernanceUsage(command)
		}
		options.governanceID = strings.TrimSpace(positional[0])
		if options.governanceID == "" {
			return options, memoryGovernanceUsage(command)
		}
		if command == "supersede" && (options.title == "" || options.content == "") {
			return options, fmt.Errorf("supersede requiere --title y --content")
		}
	case "prune":
		if len(positional) != 0 {
			return options, memoryGovernanceUsage(command)
		}
		if options.olderThan <= 0 {
			return options, fmt.Errorf("prune requiere --older-than")
		}
	default:
		return options, memoryGovernanceUsage(command)
	}
	return options, nil
}

func memoryGovernanceUsage(command string) error {
	switch command {
	case "list":
		return fmt.Errorf("uso: ai memory list --project <id> [--json]")
	case "review":
		return fmt.Errorf("uso: ai memory review <governance-id> --project <id> [--json]")
	case "delete":
		return fmt.Errorf("uso: ai memory delete <governance-id> --project <id> [--hard --yes] [--json]")
	case "supersede":
		return fmt.Errorf("uso: ai memory supersede <governance-id> --project <id> --title <texto> --content <texto> [--type tipo] [--json]")
	case "prune":
		return fmt.Errorf("uso: ai memory prune --project <id> --older-than <90d|2160h> --yes [--hard] [--json]")
	default:
		return fmt.Errorf("uso: ai memory list|review|delete|supersede|prune")
	}
}

func parseRetentionDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if strings.HasSuffix(value, "d") {
		daysText := strings.TrimSuffix(value, "d")
		days, err := strconv.Atoi(daysText)
		if err != nil || days < 1 || days > 36500 {
			return 0, fmt.Errorf("retención en días debe estar entre 1d y 36500d")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("duración de retención inválida: %s", value)
	}
	return duration, nil
}

func parseMemoryType(value string) (memory.Type, error) {
	switch memory.Type(strings.TrimSpace(strings.ToLower(value))) {
	case memory.TypeDecision:
		return memory.TypeDecision, nil
	case memory.TypeArchitecture:
		return memory.TypeArchitecture, nil
	case memory.TypeBugfix:
		return memory.TypeBugfix, nil
	case memory.TypePattern:
		return memory.TypePattern, nil
	case memory.TypeConfig:
		return memory.TypeConfig, nil
	case memory.TypeDiscovery:
		return memory.TypeDiscovery, nil
	case memory.TypeLearning:
		return memory.TypeLearning, nil
	default:
		return "", fmt.Errorf("tipo de memoria no soportado: %s", value)
	}
}

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FernandoVillegas27/ai-engineering-os/core/memory"
	engramintegration "github.com/FernandoVillegas27/ai-engineering-os/integrations/engram"
)

type memoryCommandOptions struct {
	memoryDir string
	project   string
	json      bool
	all       bool
	reason    string
	title     string
	content   string
	topic     string
	memoryType memory.Type
	retention time.Duration
}

func runMemoryCommand(args []string) error {
	if len(args) == 0 || args[0] == "status" {
		return runMemoryStatus(args)
	}
	switch args[0] {
	case "list":
		return runMemoryList(args[1:])
	case "show":
		return runMemoryShow(args[1:])
	case "delete":
		return runMemoryDelete(args[1:])
	case "supersede":
		return runMemorySupersede(args[1:])
	default:
		return fmt.Errorf("subcomando de memoria desconocido: %s", args[0])
	}
}

func runMemoryStatus(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("uso: ai memory status")
	}
	client := engramintegration.New()
	status, err := client.Status(context.Background())
	if err != nil {
		return err
	}
	root, err := resolveMemoryGovernanceDir("")
	if err != nil {
		return err
	}
	if _, err := memory.NewGovernanceStore(root); err != nil {
		return err
	}

	fmt.Println("Memoria persistente")
	fmt.Println("Gobernanza: local/auditable")
	fmt.Printf("Ledger: %s\n", root)
	fmt.Printf("Mirror: %s\n", status.Provider)
	if !status.Available {
		fmt.Println("Mirror status: no disponible (la memoria gobernada local sigue operativa)")
		return nil
	}
	if status.Healthy {
		fmt.Println("Mirror status: OK")
	} else {
		fmt.Println("Mirror status: requiere atención")
		if status.Details != "" {
			fmt.Printf("Detalle: %s\n", status.Details)
		}
	}
	if status.Version != "" {
		fmt.Printf("Versión mirror: %s\n", status.Version)
	}
	return nil
}

func runMemoryList(args []string) error {
	options, _, err := parseMemoryGovernanceArgs(args, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(options.project) == "" {
		return fmt.Errorf("--project es obligatorio")
	}
	store, err := openMemoryGovernanceStore(options.memoryDir)
	if err != nil {
		return err
	}
	values, err := store.List(options.project, options.all, time.Now().UTC())
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(values)
	}
	if len(values) == 0 {
		fmt.Println("No governed memories found.")
		return nil
	}
	for _, value := range values {
		expiry := "-"
		if value.ExpiresAt != nil {
			expiry = value.ExpiresAt.Format(time.RFC3339)
		}
		fmt.Printf("%s  %-10s  %-12s  expires=%s  %s\n",
			value.ID, value.Status, value.Type, expiry, value.Title)
	}
	return nil
}

func runMemoryShow(args []string) error {
	options, id, err := parseMemoryGovernanceArgs(args, true)
	if err != nil {
		return err
	}
	if strings.TrimSpace(options.project) == "" {
		return fmt.Errorf("--project es obligatorio")
	}
	store, err := openMemoryGovernanceStore(options.memoryDir)
	if err != nil {
		return err
	}
	record, err := store.Load(options.project, id)
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(record)
	}
	fmt.Println("Governed Memory")
	fmt.Printf("ID: %s\n", record.ID)
	fmt.Printf("Project: %s\n", record.Project)
	fmt.Printf("Status: %s\n", record.Status)
	fmt.Printf("Type: %s\n", record.Type)
	fmt.Printf("Title: %s\n", record.Title)
	fmt.Printf("Topic: %s\n", emptyDash(record.TopicKey))
	fmt.Printf("Content SHA-256: %s\n", record.ContentSHA256)
	fmt.Printf("Created: %s\n", record.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated: %s\n", record.UpdatedAt.Format(time.RFC3339))
	if record.ExpiresAt != nil {
		fmt.Printf("Expires: %s\n", record.ExpiresAt.Format(time.RFC3339))
	}
	if record.SupersededBy != "" {
		fmt.Printf("Superseded by: %s\n", record.SupersededBy)
	}
	if record.Provenance.Source != "" || record.Provenance.RunID != "" || record.Provenance.ArtifactID != "" {
		fmt.Printf("Provenance: source=%s run=%s artifact=%s task=%s agent=%s\n",
			emptyDash(record.Provenance.Source),
			emptyDash(record.Provenance.RunID),
			emptyDash(record.Provenance.ArtifactID),
			emptyDash(record.Provenance.TaskID),
			emptyDash(record.Provenance.ProducerAgent),
		)
	}
	fmt.Println("Content:")
	fmt.Println(record.Content)
	return nil
}

func runMemoryDelete(args []string) error {
	options, id, err := parseMemoryGovernanceArgs(args, true)
	if err != nil {
		return err
	}
	if strings.TrimSpace(options.project) == "" {
		return fmt.Errorf("--project es obligatorio")
	}
	store, err := openMemoryGovernanceStore(options.memoryDir)
	if err != nil {
		return err
	}
	record, err := store.Delete(options.project, id, options.reason, time.Now().UTC())
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(record)
	}
	fmt.Printf("Memory %s marked deleted. It will no longer be recalled automatically.\n", record.ID)
	return nil
}

func runMemorySupersede(args []string) error {
	options, id, err := parseMemoryGovernanceArgs(args, true)
	if err != nil {
		return err
	}
	if strings.TrimSpace(options.project) == "" {
		return fmt.Errorf("--project es obligatorio")
	}
	if strings.TrimSpace(options.content) == "" {
		return fmt.Errorf("--content es obligatorio")
	}
	store, err := openMemoryGovernanceStore(options.memoryDir)
	if err != nil {
		return err
	}
	current, err := store.Load(options.project, id)
	if err != nil {
		return err
	}
	title := strings.TrimSpace(options.title)
	if title == "" {
		title = current.Title
	}
	kind := options.memoryType
	if kind == "" {
		kind = current.Type
	}
	topic := strings.TrimSpace(options.topic)
	if topic == "" {
		topic = current.TopicKey
	}
	retention := options.retention
	if retention == 0 && current.ExpiresAt != nil {
		remaining := time.Until(*current.ExpiresAt)
		if remaining > 0 {
			retention = remaining
		}
	}
	previous, next, err := store.Supersede(options.project, id, memory.GovernedSaveRequest{
		SaveRequest: memory.SaveRequest{
			Project: options.project,
			Title: title,
			Content: options.content,
			Type: kind,
			TopicKey: topic,
			Provenance: memory.Provenance{
				Source: "manual-supersede",
			},
			Retention: retention,
		},
		Provider: "manual",
		Provenance: memory.Provenance{Source: "manual-supersede"},
		Retention: retention,
	}, time.Now().UTC())
	if err != nil {
		return err
	}
	if options.json {
		return printJSON(map[string]any{"previous": previous, "replacement": next})
	}
	fmt.Printf("Memory %s superseded by %s.\n", previous.ID, next.ID)
	return nil
}

func openMemoryGovernanceStore(explicit string) (*memory.GovernanceStore, error) {
	root, err := resolveMemoryGovernanceDir(explicit)
	if err != nil {
		return nil, err
	}
	return memory.NewGovernanceStore(root)
}

func parseMemoryGovernanceArgs(args []string, requireID bool) (memoryCommandOptions, string, error) {
	options := memoryCommandOptions{}
	var id string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--memory-dir":
			if i+1 >= len(args) { return options, "", fmt.Errorf("--memory-dir requiere ruta") }
			i++
			options.memoryDir = args[i]
		case "--project":
			if i+1 >= len(args) { return options, "", fmt.Errorf("--project requiere id") }
			i++
			options.project = args[i]
		case "--json":
			options.json = true
		case "--all":
			options.all = true
		case "--reason":
			if i+1 >= len(args) { return options, "", fmt.Errorf("--reason requiere texto") }
			i++
			options.reason = args[i]
		case "--title":
			if i+1 >= len(args) { return options, "", fmt.Errorf("--title requiere texto") }
			i++
			options.title = args[i]
		case "--content":
			if i+1 >= len(args) { return options, "", fmt.Errorf("--content requiere texto") }
			i++
			options.content = args[i]
		case "--topic":
			if i+1 >= len(args) { return options, "", fmt.Errorf("--topic requiere texto") }
			i++
			options.topic = args[i]
		case "--type":
			if i+1 >= len(args) { return options, "", fmt.Errorf("--type requiere valor") }
			i++
			options.memoryType = memory.Type(args[i])
		case "--retention":
			if i+1 >= len(args) { return options, "", fmt.Errorf("--retention requiere duración") }
			i++
			value, err := time.ParseDuration(args[i])
			if err != nil || value <= 0 {
				return options, "", fmt.Errorf("--retention debe ser una duración positiva")
			}
			options.retention = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return options, "", fmt.Errorf("opción desconocida: %s", args[i])
			}
			if id != "" {
				return options, "", fmt.Errorf("se esperaba un solo memory id")
			}
			id = args[i]
		}
	}
	if requireID && id == "" {
		return options, "", fmt.Errorf("memory id es obligatorio")
	}
	return options, id, nil
}

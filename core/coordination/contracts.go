// Package coordination defines provider-independent contracts for inter-agent work.
package coordination

import (
 "errors"
 "fmt"
 "sort"
 "strings"
)

type Role string
const (
 Architect Role = "architect"
 Backend Role = "backend-engineer"
 Frontend Role = "frontend-engineer"
 QA Role = "qa-engineer"
 Reviewer Role = "reviewer"
)

type State string
const (
 Queued State = "queued"
 Ready State = "ready"
 Running State = "running"
 Waiting State = "waiting"
 Blocked State = "blocked"
 Completed State = "completed"
 Failed State = "failed"
 Cancelled State = "cancelled"
)

type Task struct {
 ID string `json:"id"`
 Role Role `json:"role"`
 DependsOn []string `json:"depends_on,omitempty"`
 State State `json:"state"`
}

type MessageKind string
const (
 Finding MessageKind = "finding"
 Question MessageKind = "question"
 Decision MessageKind = "decision"
 Handoff MessageKind = "handoff"
)

type Message struct {
 ID string `json:"id"`
 RunID string `json:"run_id"`
 FromTask string `json:"from_task"`
 ToTask string `json:"to_task"`
 Kind MessageKind `json:"kind"`
 Body string `json:"body"`
 ArtifactIDs []string `json:"artifact_ids,omitempty"`
}

func ValidateGraph(tasks []Task) error {
 index := make(map[string]Task, len(tasks))
 for _, task := range tasks {
  if strings.TrimSpace(task.ID) == "" || strings.TrimSpace(string(task.Role)) == "" {
   return errors.New("task ID and role are required")
  }
  switch task.Role {
  case Architect, Backend, Frontend, QA, Reviewer:
  default: return fmt.Errorf("task %q: invalid role %q", task.ID, task.Role)
  }
  switch task.State {
  case Queued, Ready, Running, Waiting, Blocked, Completed, Failed, Cancelled:
  default: return fmt.Errorf("task %q: invalid state %q", task.ID, task.State)
  }
  if _, ok := index[task.ID]; ok { return fmt.Errorf("duplicate task %q", task.ID) }
  index[task.ID] = task
 }
 for _, task := range tasks {
  seen := map[string]bool{}
  for _, dep := range task.DependsOn {
   if dep == task.ID { return fmt.Errorf("task %q depends on itself", task.ID) }
   if _, ok := index[dep]; !ok { return fmt.Errorf("task %q: unknown dependency %q", task.ID, dep) }
   if seen[dep] { return fmt.Errorf("task %q: duplicate dependency %q", task.ID, dep) }
   seen[dep] = true
  }
 }
 colors := map[string]int{}
 var visit func(string) error
 visit = func(id string) error {
  if colors[id] == 1 { return fmt.Errorf("dependency cycle at %q", id) }
  if colors[id] == 2 { return nil }
  colors[id] = 1
  for _, dep := range index[id].DependsOn { if err := visit(dep); err != nil { return err } }
  colors[id] = 2
  return nil
 }
 for id := range index { if err := visit(id); err != nil { return err } }
 return nil
}

// ReadyTasks returns queued work whose dependencies have all completed.
// Failed or blocked dependencies never authorize a downstream task.
func ReadyTasks(tasks []Task) ([]Task, error) {
 if err := ValidateGraph(tasks); err != nil { return nil, err }
 index := map[string]Task{}
 for _, task := range tasks { index[task.ID] = task }
 ready := []Task{}
 for _, task := range tasks {
  if task.State != Queued { continue }
  eligible := true
  for _, dep := range task.DependsOn { if index[dep].State != Completed { eligible = false; break } }
  if eligible { ready = append(ready, task) }
 }
 sort.Slice(ready, func(i, j int) bool { return ready[i].ID < ready[j].ID })
 return ready, nil
}

// ValidateMessage rejects unscoped or fabricated communication. The sender and
// recipient must both belong to the validated run task graph.
func ValidateMessage(msg Message, tasks []Task) error {
 if err := ValidateGraph(tasks); err != nil { return err }
 if strings.TrimSpace(msg.ID) == "" || strings.TrimSpace(msg.RunID) == "" ||
  strings.TrimSpace(msg.Body) == "" || strings.TrimSpace(msg.FromTask) == "" ||
  strings.TrimSpace(msg.ToTask) == "" {
  return errors.New("message identity, run, sender, recipient and body are required")
 }
 switch msg.Kind {
 case Finding, Question, Decision, Handoff:
 default: return fmt.Errorf("invalid message kind %q", msg.Kind)
 }
 known := map[string]bool{}
 for _, task := range tasks { known[task.ID] = true }
 if !known[msg.FromTask] || !known[msg.ToTask] { return errors.New("message sender or recipient is not in task graph") }
 if msg.FromTask == msg.ToTask { return errors.New("inter-agent message cannot target its sender") }
 seen := map[string]bool{}
 for _, id := range msg.ArtifactIDs {
  if strings.TrimSpace(id) == "" || seen[id] { return errors.New("invalid or duplicate artifact reference") }
  seen[id] = true
 }
 return nil
}

package coordination

import (
 "errors"
 "fmt"
 "strings"
)

// BuildDependentInstructions prepares an agent prompt using only completed
// dependencies with persisted outputs. This is data flow, not authorization:
// provider-specific sandbox/tool approvals still apply independently.
func BuildDependentInstructions(task Task, instructions map[string]string, tasks []Task, results *DurableResultExecutor) (string,error) {
 if results==nil {return "",errors.New("durable results required")}
 base:=strings.TrimSpace(instructions[task.ID])
 if base=="" {return "",fmt.Errorf("missing instructions for %q",task.ID)}
 states:=make(map[string]Task,len(tasks))
 for _,candidate:=range tasks {states[candidate.ID]=candidate}
 var prompt strings.Builder
 prompt.WriteString(base)
 for _,dependency:=range task.DependsOn {
  predecessor,ok:=states[dependency]
  if !ok || predecessor.State!=Completed {return "",fmt.Errorf("dependency %q not completed",dependency)}
  result,err:=results.Read(dependency)
  if err!=nil {return "",fmt.Errorf("dependency %q result unavailable: %w",dependency,err)}
  if result.Role!=predecessor.Role {return "",fmt.Errorf("dependency %q result role mismatch",dependency)}
  prompt.WriteString("\n\n--- BEGIN UNTRUSTED AGENT RESULT: ")
  prompt.WriteString(dependency)
  prompt.WriteString(" ---\n")
  prompt.WriteString(result.Output)
  prompt.WriteString("\n--- END UNTRUSTED AGENT RESULT ---\n")
 }
 return prompt.String(),nil
}

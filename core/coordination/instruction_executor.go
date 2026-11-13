package coordination

import (
 "context"
 "errors"
 "fmt"
 "strings"
)

// InstructionExecutor resolves real task instructions before starting an agent.
// It intentionally rejects missing instructions rather than reporting a role
// label as completed work. The underlying CLI still requires a separately
// approved sandbox and provider-specific invocation.
type InstructionExecutor struct {
 Command *CommandExecutor
 Instructions map[string]string
}

func (e *InstructionExecutor) Execute(ctx context.Context, task Task) error {
 if e==nil || e.Command==nil {return errors.New("command executor required")}
 instruction,ok:=e.Instructions[task.ID]
 if !ok || strings.TrimSpace(instruction)=="" {
  return fmt.Errorf("task %q has no executable instructions",task.ID)
 }
 if err:=ctx.Err();err!=nil{return err}
 return e.Command.executeWithPrompt(ctx,task,fmt.Sprintf(
  "Role: %s\nTask ID: %s\nInstructions:\n%s\n\nFollow the host's sandbox, tool and approval policies. Do not bypass approval gates.\n",
  task.Role,task.ID,instruction))
}

package coordination

import (
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "strings"
)

// CommandExecutor bridges a trusted, preconfigured agent CLI to Executor.
// The caller chooses the executable and fixed arguments for its installed
// Cursor/Codex version. It never invokes a shell or grants tool permissions.
// Task identity and role are supplied on stdin, not interpolated into flags.
// Configure a sandboxed working directory and explicit approval policy first.
type CommandExecutor struct {
 Binary string
 Args []string
 Workdir string
 Output io.Writer
}

func (e *CommandExecutor) Execute(ctx context.Context, task Task) error {
 return errors.New("agent instructions required: use InstructionExecutor")
}

func (e *CommandExecutor) executeWithPrompt(ctx context.Context, task Task, prompt string) error {
 if e==nil || strings.TrimSpace(e.Binary)=="" || strings.TrimSpace(e.Workdir)=="" {
  return errors.New("agent binary and workdir required")
 }
 if err:=ctx.Err();err!=nil{return err}
 dir,err:=os.Stat(e.Workdir)
 if err!=nil{return fmt.Errorf("agent workdir: %w",err)}
 if !dir.IsDir(){return errors.New("agent workdir must be a directory")}
 if strings.ContainsAny(e.Binary,"\r\n") {return errors.New("invalid agent executable")}
 // Task fields are data only. A trusted controller must provide the actual
 // task instructions and tool policy through its own runtime adapter.
 cmd:=exec.CommandContext(ctx,e.Binary,e.Args...)
 cmd.Dir=e.Workdir
 cmd.Stdin=strings.NewReader(prompt)
 if e.Output!=nil {
  cmd.Stdout=e.Output
  cmd.Stderr=e.Output
 } else {
  cmd.Stdout=io.Discard
  cmd.Stderr=io.Discard
 }
 if err:=cmd.Run();err!=nil{return fmt.Errorf("agent task %q: %w",task.ID,err)}
 return nil
}

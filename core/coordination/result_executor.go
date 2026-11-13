package coordination

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "io"
 "strings"
 "sync"
)

// AgentResult is a bounded record of the CLI's response. A successful process
// exit is not proof of correct code or permission to merge.
type AgentResult struct {
 TaskID string
 Role Role
 Output string
}

// ResultExecutor retains per-task agent output for explicit downstream
// inspection. It rejects duplicate task IDs and excessive output rather than
// silently truncating evidence. One instance must be owned by a single run.
type ResultExecutor struct {
 Command *CommandExecutor
 Instructions map[string]string
 MaxOutputBytes int
 mu sync.Mutex
 results map[string]AgentResult
}

type limitedAgentWriter struct {
 buf bytes.Buffer
 limit int
 overflow bool
}
func (w *limitedAgentWriter) Write(p []byte)(int,error){
 if len(p)>w.limit-w.buf.Len(){
  w.overflow=true
  return 0,errors.New("agent output exceeds configured limit")
 }
 return w.buf.Write(p)
}

func (e *ResultExecutor) Execute(ctx context.Context,task Task)error{
 if e==nil||e.Command==nil{return errors.New("agent command required")}
 instruction:=e.Instructions[task.ID]
 if strings.TrimSpace(instruction)=="" {return fmt.Errorf("missing instructions for task %q",task.ID)}
 if e.MaxOutputBytes<=0{return errors.New("positive agent output limit required")}
 e.mu.Lock()
 defer e.mu.Unlock()
 if e.results==nil {e.results=map[string]AgentResult{}}
 if _,exists:=e.results[task.ID];exists{return fmt.Errorf("agent task %q already has a recorded result",task.ID)}
 capture:=&limitedAgentWriter{limit:e.MaxOutputBytes}
 command:=*e.Command
 command.Output=io.Writer(capture)
 prompt:=fmt.Sprintf("Role: %s\nTask ID: %s\nInstructions:\n%s\n\nRespect host sandbox and approval gates.\n",task.Role,task.ID,instruction)
 if err:=command.executeWithPrompt(ctx,task,prompt);err!=nil{return err}
 if capture.overflow{return fmt.Errorf("agent output exceeded %d bytes",e.MaxOutputBytes)}
 if strings.TrimSpace(capture.buf.String())=="" {return fmt.Errorf("agent task %q returned empty output",task.ID)}
 e.results[task.ID]=AgentResult{TaskID:task.ID,Role:task.Role,Output:capture.buf.String()}
 return nil
}

func (e *ResultExecutor) Result(taskID string)(AgentResult,bool){
 if e==nil{return AgentResult{},false}
 e.mu.Lock();defer e.mu.Unlock()
 result,ok:=e.results[taskID]
 return result,ok
}

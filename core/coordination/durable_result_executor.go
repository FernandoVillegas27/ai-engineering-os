package coordination

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
)

// DurableResultExecutor persists agent output before reporting a task as
// successful. The directory must be owned by the run lease holder.
// Results are immutable: an existing result requires explicit reconciliation.
type DurableResultExecutor struct {
 Agent *ResultExecutor
 Directory string
}

func resultPath(directory, taskID string) string {
 // Task IDs are untrusted; do not use them as path components.
 return filepath.Join(directory,fmt.Sprintf("%x.json",[]byte(taskID)))
}

func (e *DurableResultExecutor) Execute(ctx context.Context,task Task) error {
 if e==nil||e.Agent==nil||strings.TrimSpace(e.Directory)=="" {return errors.New("durable result executor requires agent and directory")}
 if strings.TrimSpace(task.ID)=="" {return errors.New("task ID required")}
 info,err:=os.Lstat(e.Directory)
 if err!=nil{return fmt.Errorf("result directory: %w",err)}
 if !info.IsDir(){return errors.New("result directory must be a real directory")}
 path:=resultPath(e.Directory,task.ID)
 if _,err:=os.Lstat(path);err==nil{return fmt.Errorf("task %q has a durable result; reconcile before retry",task.ID)}else if !errors.Is(err,os.ErrNotExist){return err}
 if err:=e.Agent.Execute(ctx,task);err!=nil{return err}
 result,ok:=e.Agent.Result(task.ID)
 if !ok{return errors.New("agent returned no result")}
 encoded,err:=json.Marshal(result)
 if err!=nil{return err}
 file,err:=os.OpenFile(path,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{return fmt.Errorf("persist result: %w",err)}
 defer file.Close()
 if _,err=file.Write(append(encoded,'\n'));err!=nil{return err}
 if err=file.Sync();err!=nil{return err}
 return nil
}

func (e *DurableResultExecutor) Read(taskID string)(AgentResult,error){
 if e==nil||strings.TrimSpace(e.Directory)==""||strings.TrimSpace(taskID)=="" {return AgentResult{},errors.New("result directory and task ID required")}
 data,err:=os.ReadFile(resultPath(e.Directory,taskID))
 if err!=nil{return AgentResult{},err}
 var result AgentResult
 if err:=json.Unmarshal(data,&result);err!=nil{return AgentResult{},err}
 if result.TaskID!=taskID||strings.TrimSpace(result.Output)=="" {return AgentResult{},errors.New("invalid durable agent result")}
 return result,nil
}

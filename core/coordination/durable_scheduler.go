package coordination

import (
 "bufio"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "sync"
)

// TaskEvent is a durable state transition for a single run.
type TaskEvent struct {
 RunID string `json:"run_id"`
 TaskID string `json:"task_id"`
 From State `json:"from"`
 To State `json:"to"`
}

// DurableScheduler replays task transitions from an append-only journal.
// Its lock is process-local. The run store must ensure single-writer ownership
// of this journal across processes.
type DurableScheduler struct {
 mu sync.Mutex
 runID string
 path string
 scheduler *Scheduler
}

func NewDurableScheduler(runID,path string,tasks []Task) (*DurableScheduler,error) {
 if runID=="" || path=="" {return nil,errors.New("run ID and task journal path required")}
 s,err:=NewScheduler(tasks)
 if err!=nil{return nil,err}
 d:=&DurableScheduler{runID:runID,path:path,scheduler:s}
 f,err:=os.Open(path)
 if errors.Is(err,os.ErrNotExist) {return d,nil}
 if err!=nil{return nil,err}
 defer f.Close()
 scanner:=bufio.NewScanner(f)
 scanner.Buffer(make([]byte,64*1024),4*1024*1024)
 for scanner.Scan() {
  var event TaskEvent
  if err:=json.Unmarshal(scanner.Bytes(),&event);err!=nil {return nil,fmt.Errorf("corrupt task journal: %w",err)}
  if event.RunID!=runID {return nil,errors.New("foreign run in task journal")}
  before:=s.Snapshot()
  found:=false
  for _,task:=range before {
   if task.ID==event.TaskID {
    found=true
    if task.State!=event.From {return nil,fmt.Errorf("task %q journal state mismatch",task.ID)}
   }
  }
  if !found {return nil,fmt.Errorf("unknown journal task %q",event.TaskID)}
  if err:=s.Transition(event.TaskID,event.To);err!=nil{return nil,fmt.Errorf("invalid replay transition: %w",err)}
 }
 if err:=scanner.Err();err!=nil{return nil,err}
 return d,nil
}

func (d *DurableScheduler) Snapshot() []Task {
 d.mu.Lock()
 defer d.mu.Unlock()
 return d.scheduler.Snapshot()
}

func (d *DurableScheduler) Transition(id string,next State) error {
 d.mu.Lock()
 defer d.mu.Unlock()
 // Validate on an isolated clone before writing; only accepted transitions
 // may enter the durable journal.
 before:=d.scheduler.Snapshot()
 trial,err:=NewScheduler(before)
 if err!=nil{return err}
 if err:=trial.Transition(id,next);err!=nil{return err}
 var from State
 for _,task:=range before {if task.ID==id {from=task.State;break}}
 event:=TaskEvent{RunID:d.runID,TaskID:id,From:from,To:next}
 data,err:=json.Marshal(event)
 if err!=nil{return err}
 f,err:=os.OpenFile(d.path,os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600)
 if err!=nil{return err}
 if _,err=f.Write(append(data,'\n'));err!=nil {f.Close();return err}
 if err=f.Sync();err!=nil {f.Close();return err}
 if err=f.Close();err!=nil{return err}
 return d.scheduler.Transition(id,next)
}

package coordination

import (
 "errors"
 "fmt"
 "sync"
)

// Scheduler owns the in-memory task graph for one run. Every transition is
// validated; persistence and cross-process coordination remain the run store's
// responsibility. It never grants tools or promotes repository changes.
type Scheduler struct {
 mu sync.Mutex
 tasks []Task
}

func NewScheduler(tasks []Task) (*Scheduler,error) {
 if len(tasks)==0 {return nil,errors.New("scheduler requires tasks")}
 if err:=ValidateGraph(tasks);err!=nil{return nil,err}
 cloned:=cloneTasks(tasks)
 return &Scheduler{tasks:cloned},nil
}

func cloneTasks(tasks []Task) []Task {
 out:=make([]Task,len(tasks))
 for i,task:=range tasks {
  out[i]=task
  out[i].DependsOn=append([]string(nil),task.DependsOn...)
 }
 return out
}

func (s *Scheduler) Snapshot() []Task {
 s.mu.Lock()
 defer s.mu.Unlock()
 return cloneTasks(s.tasks)
}

func (s *Scheduler) Ready() ([]Task,error) {
 s.mu.Lock()
 defer s.mu.Unlock()
 return ReadyTasks(s.tasks)
}

// Transition rejects unknown state edges and dependency bypass. Ready is only
// reachable from queued when every prerequisite has completed.
func (s *Scheduler) Transition(id string,next State) error {
 s.mu.Lock()
 defer s.mu.Unlock()
 idx:=-1
 for i:=range s.tasks {if s.tasks[i].ID==id {idx=i;break}}
 if idx<0{return fmt.Errorf("unknown task %q",id)}
 current:=s.tasks[idx].State
 allowed:=map[State]map[State]bool{
  Queued:{Ready:true,Cancelled:true},
  Ready:{Running:true,Blocked:true,Cancelled:true},
  Running:{Waiting:true,Blocked:true,Completed:true,Failed:true,Cancelled:true},
  Waiting:{Running:true,Blocked:true,Cancelled:true},
  Blocked:{Ready:true,Cancelled:true},
  Failed:{Ready:true,Cancelled:true},
 }
 if !allowed[current][next] {return fmt.Errorf("invalid transition %q -> %q",current,next)}
 if next==Ready || next==Running {
  index:=map[string]State{}
  for _,task:=range s.tasks {index[task.ID]=task.State}
  for _,dep:=range s.tasks[idx].DependsOn {
   if index[dep]!=Completed {return fmt.Errorf("task %q blocked by dependency %q",id,dep)}
  }
 }
 s.tasks[idx].State=next
 return nil
}

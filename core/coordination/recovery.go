package coordination

import (
 "errors"
 "fmt"
)

// RecoverInterrupted explicitly quarantines work left Running after a crash.
// It never retries or marks such work successful. An operator or control plane
// must inspect artifacts and authorize a subsequent Blocked -> Ready transition.
// The caller must own the run's exclusive writer lease.
func RecoverInterrupted(scheduler *DurableScheduler) ([]string,error) {
 if scheduler==nil {return nil,errors.New("scheduler required")}
 interrupted:=[]string{}
 for _,task:=range scheduler.Snapshot() {
  if task.State!=Running {continue}
  if err:=scheduler.Transition(task.ID,Blocked);err!=nil {
   return interrupted,fmt.Errorf("quarantine interrupted task %q: %w",task.ID,err)
  }
  interrupted=append(interrupted,task.ID)
 }
 return interrupted,nil
}

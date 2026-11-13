package coordination

import (
 "errors"
 "fmt"
 "os"
)

// ResultReconciliation describes a durable task/result discrepancy.
// It is diagnostic only: a result file does not prove side effects succeeded,
// and no state transition or retry is authorized by this report.
type ResultReconciliation struct {
 TaskID string
 State State
 HasResult bool
 RequiresReview bool
}

// InspectDurableResults compares the task journal against immutable result
// records. The caller must hold the run lease to prevent concurrent changes.
// Corrupt records fail closed rather than being treated as missing results.
func InspectDurableResults(scheduler *DurableScheduler, store *DurableResultExecutor) ([]ResultReconciliation,error) {
 if scheduler==nil || store==nil {return nil,errors.New("scheduler and result store required")}
 report:=[]ResultReconciliation{}
 for _,task:=range scheduler.Snapshot() {
  _,err:=store.Read(task.ID)
  hasResult:=err==nil
  if err!=nil && !errors.Is(err,os.ErrNotExist) {
   return nil,fmt.Errorf("inspect result for task %q: %w",task.ID,err)
  }
  // A completed task without a result is also inconsistent when this
  // result store is the configured executor for the entire run.
  needsReview:=(hasResult && task.State!=Completed) || (!hasResult && task.State==Completed) || task.State==Running
  report=append(report,ResultReconciliation{
   TaskID:task.ID,State:task.State,HasResult:hasResult,RequiresReview:needsReview,
  })
 }
 return report,nil
}

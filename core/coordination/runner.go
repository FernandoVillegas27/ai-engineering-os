package coordination

import (
 "context"
 "errors"
 "fmt"
)

// Executor is a runtime-neutral adapter. The caller is responsible for
// sandboxing, tool permissions, workspace isolation and human approvals.
// A successful return means only that the role's work finished, not that
// repository changes are approved or may be merged.
type Executor interface {
 Execute(context.Context, Task) error
}

// RunReady executes eligible tasks serially, preserving deterministic order.
// It deliberately does not automatically retry failures or bypass human gates.
// The caller must hold exclusive ownership of the durable run journals.
func RunReady(ctx context.Context, scheduler *DurableScheduler, executor Executor) error {
 if scheduler==nil || executor==nil {return errors.New("scheduler and executor required")}
 if err:=ctx.Err();err!=nil {return err}
 tasks:=scheduler.Snapshot()
 ready,err:=ReadyTasks(tasks)
 if err!=nil {return err}
 for _,task:=range ready {
  if err:=ctx.Err();err!=nil {return err}
  if err:=scheduler.Transition(task.ID,Ready);err!=nil {return err}
 }
 for _,task:=range scheduler.Snapshot() {
  if task.State!=Ready {continue}
  if err:=ctx.Err();err!=nil {return err}
  if err:=scheduler.Transition(task.ID,Running);err!=nil {return err}
  if err:=executor.Execute(ctx,task);err!=nil {
   if transitionErr:=scheduler.Transition(task.ID,Failed);transitionErr!=nil {
    return fmt.Errorf("task %s execution failed: %v; recording failure: %w",task.ID,err,transitionErr)
   }
   return fmt.Errorf("task %s: %w",task.ID,err)
  }
  if err:=ctx.Err();err!=nil {
   // Preserve Running for explicit recovery policy rather than falsely
   // recording success or failure after cancellation.
   return err
  }
  if err:=scheduler.Transition(task.ID,Completed);err!=nil {return err}
 }
 return nil
}

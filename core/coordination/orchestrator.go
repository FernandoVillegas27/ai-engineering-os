package coordination

import (
 "context"
 "errors"
 "fmt"
)

// RunUntilIdle advances a durable dependency graph until all work completes
// or no eligible work remains. It never retries failed, blocked, waiting or
// interrupted tasks, and does not authorize tool access or repository merges.
// Callers must enforce an exclusive run writer and agent tool policy.
func RunUntilIdle(ctx context.Context, scheduler *DurableScheduler, executor Executor) error {
 if scheduler==nil || executor==nil {return errors.New("scheduler and executor required")}
 for {
  if err:=ctx.Err();err!=nil {return err}
  before:=scheduler.Snapshot()
  completed:=true
  runnable:=false
  for _,task:=range before {
   if task.State!=Completed && task.State!=Cancelled {completed=false}
   if task.State==Ready {runnable=true}
  }
  if completed {return nil}
  eligible,err:=ReadyTasks(before)
  if err!=nil {return err}
  if len(eligible)==0 && !runnable {
   return fmt.Errorf("run stalled: unfinished tasks have no eligible execution path")
  }
  if err:=RunReady(ctx,scheduler,executor);err!=nil {return err}
 }
}

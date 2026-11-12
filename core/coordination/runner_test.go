package coordination

import (
 "context"
 "errors"
 "path/filepath"
 "testing"
)

type recordingExecutor struct {
 called []string
 fail string
}
func (e *recordingExecutor) Execute(_ context.Context, task Task) error {
 e.called=append(e.called,task.ID)
 if task.ID==e.fail {return errors.New("provider failure")}
 return nil
}

func TestRunReadyExecutesDependencyChainAcrossRestarts(t *testing.T) {
 tasks:=[]Task{
  {ID:"architect",Role:Architect,State:Queued},
  {ID:"backend",Role:Backend,State:Queued,DependsOn:[]string{"architect"}},
  {ID:"qa",Role:QA,State:Queued,DependsOn:[]string{"backend"}},
 }
 path:=filepath.Join(t.TempDir(),"tasks.jsonl")
 exec:=&recordingExecutor{}
 for range tasks {
  scheduler,err:=NewDurableScheduler("run",path,tasks)
  if err!=nil {t.Fatal(err)}
  if err:=RunReady(context.Background(),scheduler,exec);err!=nil {t.Fatal(err)}
 }
 if len(exec.called)!=3 || exec.called[0]!="architect" || exec.called[1]!="backend" || exec.called[2]!="qa" {
  t.Fatalf("wrong execution sequence: %v",exec.called)
 }
 scheduler,err:=NewDurableScheduler("run",path,tasks)
 if err!=nil {t.Fatal(err)}
 for _,task:=range scheduler.Snapshot() {
  if task.State!=Completed {t.Fatalf("task not persisted: %#v",task)}
 }
 if err:=RunReady(context.Background(),scheduler,exec);err!=nil {t.Fatal(err)}
 if len(exec.called)!=3 {t.Fatal("completed tasks re-executed")}
}

func TestRunReadyFailsClosedOnExecutorFailure(t *testing.T) {
 tasks:=[]Task{{ID:"architect",Role:Architect,State:Queued},{ID:"backend",Role:Backend,State:Queued,DependsOn:[]string{"architect"}}}
 path:=filepath.Join(t.TempDir(),"tasks.jsonl")
 scheduler,err:=NewDurableScheduler("run",path,tasks)
 if err!=nil {t.Fatal(err)}
 exec:=&recordingExecutor{fail:"architect"}
 if err:=RunReady(context.Background(),scheduler,exec);err==nil {t.Fatal("executor failure accepted")}
 reopened,err:=NewDurableScheduler("run",path,tasks)
 if err!=nil {t.Fatal(err)}
 state:=reopened.Snapshot()
 if state[0].State!=Failed || state[1].State!=Queued {t.Fatalf("failed task unlocked dependent work: %#v",state)}
 if err:=RunReady(context.Background(),reopened,exec);err!=nil {t.Fatal(err)}
 if len(exec.called)!=1 {t.Fatal("failed task retried without explicit authorization")}
}

func TestRunReadyHonorsCancellation(t *testing.T) {
 tasks:=[]Task{{ID:"architect",Role:Architect,State:Queued}}
 scheduler,err:=NewDurableScheduler("run",filepath.Join(t.TempDir(),"tasks.jsonl"),tasks)
 if err!=nil {t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 exec:=&recordingExecutor{}
 if err:=RunReady(ctx,scheduler,exec);!errors.Is(err,context.Canceled) {t.Fatalf("expected cancellation, got %v",err)}
 if len(exec.called)!=0 || scheduler.Snapshot()[0].State!=Queued {t.Fatal("cancelled run mutated state")}
}

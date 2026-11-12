package coordination

import (
 "context"
 "path/filepath"
 "testing"
)

func TestRunUntilIdleExecutesFiveRolesWithDependencies(t *testing.T) {
 tasks:=[]Task{
  {ID:"plan",Role:Architect,State:Queued},
  {ID:"backend",Role:Backend,State:Queued,DependsOn:[]string{"plan"}},
  {ID:"frontend",Role:Frontend,State:Queued,DependsOn:[]string{"plan"}},
  {ID:"qa",Role:QA,State:Queued,DependsOn:[]string{"backend","frontend"}},
  {ID:"review",Role:Reviewer,State:Queued,DependsOn:[]string{"qa"}},
 }
 path:=filepath.Join(t.TempDir(),"tasks.jsonl")
 scheduler,err:=NewDurableScheduler("run",path,tasks)
 if err!=nil {t.Fatal(err)}
 exec:=&recordingExecutor{}
 if err:=RunUntilIdle(context.Background(),scheduler,exec);err!=nil {t.Fatal(err)}
 want:=[]string{"plan","backend","frontend","qa","review"}
 if len(exec.called)!=len(want) {t.Fatalf("executed %v",exec.called)}
 for i,id:=range want {if exec.called[i]!=id {t.Fatalf("execution order %v, want %v",exec.called,want)}}
 reopened,err:=NewDurableScheduler("run",path,tasks)
 if err!=nil {t.Fatal(err)}
 if err:=RunUntilIdle(context.Background(),reopened,exec);err!=nil {t.Fatal(err)}
 if len(exec.called)!=len(want) {t.Fatal("completed work executed twice")}
}

func TestRunUntilIdleStopsAtFailedDependency(t *testing.T) {
 tasks:=[]Task{{ID:"plan",Role:Architect,State:Queued},{ID:"qa",Role:QA,State:Queued,DependsOn:[]string{"plan"}}}
 scheduler,err:=NewDurableScheduler("run",filepath.Join(t.TempDir(),"tasks.jsonl"),tasks)
 if err!=nil {t.Fatal(err)}
 exec:=&recordingExecutor{fail:"plan"}
 if err:=RunUntilIdle(context.Background(),scheduler,exec);err==nil {t.Fatal("executor failure ignored")}
 if len(exec.called)!=1 {t.Fatalf("dependent task executed: %v",exec.called)}
 exec.fail=""
 if err:=RunUntilIdle(context.Background(),scheduler,exec);err==nil {t.Fatal("stalled run reported success")}
 if len(exec.called)!=1 {t.Fatal("failed work retried automatically")}
}

func TestRunUntilIdleDoesNotReportCancelledWorkAsSuccess(t *testing.T) {
 tasks:=[]Task{{ID:"plan",Role:Architect,State:Queued},{ID:"qa",Role:QA,State:Queued,DependsOn:[]string{"plan"}}}
 scheduler,err:=NewDurableScheduler("run",filepath.Join(t.TempDir(),"tasks.jsonl"),tasks)
 if err!=nil {t.Fatal(err)}
 if err:=scheduler.Transition("plan",Cancelled);err!=nil {t.Fatal(err)}
 if err:=scheduler.Transition("qa",Cancelled);err!=nil {t.Fatal(err)}
 exec:=&recordingExecutor{}
 if err:=RunUntilIdle(context.Background(),scheduler,exec);err==nil {
  t.Fatal("cancelled tasks must not produce successful run")
 }
 if len(exec.called)!=0 {t.Fatalf("cancelled work executed: %v",exec.called)}
}

func TestRunUntilIdleRejectsPartialCancellation(t *testing.T) {
 tasks:=[]Task{{ID:"plan",Role:Architect,State:Queued},{ID:"qa",Role:QA,State:Queued,DependsOn:[]string{"plan"}}}
 scheduler,err:=NewDurableScheduler("run",filepath.Join(t.TempDir(),"tasks.jsonl"),tasks)
 if err!=nil {t.Fatal(err)}
 if err:=scheduler.Transition("plan",Cancelled);err!=nil {t.Fatal(err)}
 exec:=&recordingExecutor{}
 if err:=RunUntilIdle(context.Background(),scheduler,exec);err==nil {t.Fatal("partial cancellation reported success")}
 if len(exec.called)!=0 {t.Fatal("cancelled dependency bypassed")}
}

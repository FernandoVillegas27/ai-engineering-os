package coordination

import "testing"

func TestSchedulerEndToEndHandoff(t *testing.T) {
 s,err:=NewScheduler([]Task{
  {ID:"architect",Role:Architect,State:Queued},
  {ID:"backend",Role:Backend,State:Queued,DependsOn:[]string{"architect"}},
  {ID:"qa",Role:QA,State:Queued,DependsOn:[]string{"backend"}},
  {ID:"review",Role:Reviewer,State:Queued,DependsOn:[]string{"qa"}},
 })
 if err!=nil {t.Fatal(err)}
 for _,id:=range []string{"architect","backend","qa","review"} {
  if err:=s.Transition(id,Ready);err!=nil {t.Fatalf("ready %s: %v",id,err)}
  if err:=s.Transition(id,Running);err!=nil {t.Fatalf("run %s: %v",id,err)}
  if err:=s.Transition(id,Completed);err!=nil {t.Fatalf("complete %s: %v",id,err)}
 }
 tasks:=s.Snapshot()
 for _,task:=range tasks {if task.State!=Completed {t.Fatalf("task %s not completed",task.ID)}}
}
func TestSchedulerRejectsDependencyBypassAndInvalidTransitions(t *testing.T) {
 s,err:=NewScheduler([]Task{{ID:"a",Role:Architect,State:Queued},{ID:"b",Role:Backend,State:Queued,DependsOn:[]string{"a"}}})
 if err!=nil {t.Fatal(err)}
 if err:=s.Transition("b",Ready);err==nil {t.Fatal("dependency bypass accepted")}
 if err:=s.Transition("a",Completed);err==nil {t.Fatal("queued task completed directly")}
 if err:=s.Transition("unknown",Running);err==nil {t.Fatal("unknown task accepted")}
 if err:=s.Transition("a",Ready);err!=nil {t.Fatal(err)}
 if err:=s.Transition("a",Running);err!=nil {t.Fatal(err)}
 if err:=s.Transition("a",Failed);err!=nil {t.Fatal(err)}
 if err:=s.Transition("b",Ready);err==nil {t.Fatal("failed dependency accepted")}
 if err:=s.Transition("a",Ready);err!=nil {t.Fatal(err)}
 if err:=s.Transition("a",Running);err!=nil {t.Fatal(err)}
 if err:=s.Transition("a",Completed);err!=nil {t.Fatal(err)}
 if err:=s.Transition("b",Ready);err!=nil {t.Fatal(err)}
 if err:=s.Transition("b",Completed);err==nil {t.Fatal("skipped execution accepted")}
}
func TestSchedulerSnapshotIsolation(t *testing.T) {
 s,err:=NewScheduler([]Task{{ID:"a",Role:Architect,State:Queued},{ID:"b",Role:QA,State:Queued,DependsOn:[]string{"a"}}})
 if err!=nil {t.Fatal(err)}
 snapshot:=s.Snapshot()
 snapshot[0].State=Completed
 snapshot[1].DependsOn[0]="unknown"
 if err:=s.Transition("b",Ready);err==nil {t.Fatal("snapshot mutation bypassed dependency")}
}

package coordination

import "testing"

func TestReadyTasksEnforcesDependenciesAndOrdering(t *testing.T) {
 tasks := []Task{
  {ID:"review",Role:Reviewer,State:Queued,DependsOn:[]string{"api","ui"}},
  {ID:"ui",Role:Frontend,State:Completed},
  {ID:"api",Role:Backend,State:Completed},
  {ID:"plan",Role:Architect,State:Queued},
 }
 ready,err:=ReadyTasks(tasks)
 if err!=nil { t.Fatal(err) }
 if len(ready)!=2 || ready[0].ID!="plan" || ready[1].ID!="review" { t.Fatalf("unexpected ready tasks: %#v",ready) }
 tasks[2].State=Failed
 ready,err=ReadyTasks(tasks)
 if err!=nil { t.Fatal(err) }
 if len(ready)!=1 || ready[0].ID!="plan" { t.Fatalf("failed dependency must block review: %#v",ready) }
}
func TestGraphRejectsCyclesMissingAndDuplicateDependencies(t *testing.T) {
 cases:=[][]Task{
  {{ID:"a",Role:Architect,State:Queued,DependsOn:[]string{"b"}},{ID:"b",Role:QA,State:Queued,DependsOn:[]string{"a"}}},
  {{ID:"a",Role:Architect,State:Queued,DependsOn:[]string{"missing"}}},
  {{ID:"a",Role:Architect,State:Queued},{ID:"a",Role:QA,State:Queued}},
  {{ID:"a",Role:Architect,State:Queued,DependsOn:[]string{"a"}}},
  {{ID:"a",Role:Architect,State:Queued},{ID:"b",Role:QA,State:Queued,DependsOn:[]string{"a","a"}}},
 }
 for i,tasks:=range cases { if err:=ValidateGraph(tasks);err==nil { t.Fatalf("case %d: expected rejection",i) } }
}
func TestMessageValidation(t *testing.T) {
 tasks:=[]Task{{ID:"architect",Role:Architect,State:Running},{ID:"backend",Role:Backend,State:Queued}}
 msg:=Message{ID:"m1",RunID:"run1",FromTask:"architect",ToTask:"backend",Kind:Handoff,Body:"Implement API",ArtifactIDs:[]string{"design-1"}}
 if err:=ValidateMessage(msg,tasks);err!=nil { t.Fatal(err) }
 cases:=[]Message{
  {ID:"m2",RunID:"run1",FromTask:"architect",ToTask:"missing",Kind:Handoff,Body:"x"},
  {ID:"m2",RunID:"run1",FromTask:"architect",ToTask:"architect",Kind:Handoff,Body:"x"},
  {ID:"m2",RunID:"run1",FromTask:"architect",ToTask:"backend",Kind:"arbitrary",Body:"x"},
  {ID:"m2",RunID:"run1",FromTask:"architect",ToTask:"backend",Kind:Question,Body:"  "},
  {ID:"m2",RunID:"run1",FromTask:"architect",ToTask:"backend",Kind:Finding,Body:"x",ArtifactIDs:[]string{"dup","dup"}},
 }
 for i,bad:=range cases { if err:=ValidateMessage(bad,tasks);err==nil { t.Fatalf("case %d: expected rejection",i) } }
}

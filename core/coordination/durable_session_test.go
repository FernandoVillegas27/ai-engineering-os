package coordination

import (
 "path/filepath"
 "testing"
)

func TestDurableSessionRestoresTaskAndHandoff(t *testing.T) {
 tasks:=[]Task{{ID:"architect",Role:Architect,State:Queued},{ID:"backend",Role:Backend,State:Queued,DependsOn:[]string{"architect"}}}
 dir:=t.TempDir()
 taskPath:=filepath.Join(dir,"tasks.jsonl")
 messagePath:=filepath.Join(dir,"messages.jsonl")
 s,err:=OpenDurableSession("run-1",taskPath,messagePath,tasks)
 if err!=nil {t.Fatal(err)}
 msg:=Message{ID:"handoff-1",RunID:"run-1",FromTask:"architect",ToTask:"backend",Kind:Handoff,Body:"Approved design"}
 if err:=s.Send(msg);err==nil {t.Fatal("premature handoff accepted")}
 for _,state:=range []State{Ready,Running,Completed} {
  if err:=s.Tasks.Transition("architect",state);err!=nil {t.Fatal(err)}
 }
 if err:=s.Send(msg);err==nil {t.Fatal("handoff before recipient readiness accepted")}
 if err:=s.Tasks.Transition("backend",Ready);err!=nil {t.Fatal(err)}
 if err:=s.Send(msg);err!=nil {t.Fatal(err)}
 restored,err:=OpenDurableSession("run-1",taskPath,messagePath,tasks)
 if err!=nil {t.Fatal(err)}
 state:=restored.Tasks.Snapshot()
 if state[0].State!=Completed || state[1].State!=Ready {t.Fatalf("task replay lost: %#v",state)}
 messages,err:=restored.Messages.Replay()
 if err!=nil || len(messages)!=1 || messages[0].ID!="handoff-1" {t.Fatalf("message replay lost: %#v %v",messages,err)}
 if err:=restored.Send(msg);err==nil {t.Fatal("duplicate message accepted")}
}
func TestDurableSessionRejectsJournalPathCollision(t *testing.T) {
 tasks:=[]Task{{ID:"a",Role:Architect,State:Queued}}
 path:=filepath.Join(t.TempDir(),"same.jsonl")
 if _,err:=OpenDurableSession("run",path,path,tasks);err==nil {t.Fatal("shared journal path accepted")}
}

package coordination

import (
 "path/filepath"
 "testing"
)

func TestSessionDurableAgentHandoff(t *testing.T) {
 tasks:=[]Task{{ID:"architect",Role:Architect,State:Queued},{ID:"backend",Role:Backend,State:Queued,DependsOn:[]string{"architect"}}}
 path:=filepath.Join(t.TempDir(),"messages.jsonl")
 s,err:=NewSession("run-1",path,tasks)
 if err!=nil {t.Fatal(err)}
 msg:=Message{ID:"handoff-1",RunID:"run-1",FromTask:"architect",ToTask:"backend",Kind:Handoff,Body:"Implement approved API"}
 if err:=s.Handoff(msg);err==nil {t.Fatal("handoff before sender completion accepted")}
 if err:=s.Scheduler.Transition("architect",Ready);err!=nil {t.Fatal(err)}
 if err:=s.Scheduler.Transition("architect",Running);err!=nil {t.Fatal(err)}
 if err:=s.Scheduler.Transition("architect",Completed);err!=nil {t.Fatal(err)}
 if err:=s.Handoff(msg);err!=nil {t.Fatal(err)}
 snapshot:=s.Scheduler.Snapshot()
 if snapshot[1].State!=Ready {t.Fatalf("recipient not ready: %#v",snapshot)}
 restored,err:=NewSession("run-1",path,tasks)
 if err!=nil {t.Fatal(err)}
 messages,err:=restored.Messages.Replay()
 if err!=nil || len(messages)!=1 || messages[0].ID!="handoff-1" {t.Fatalf("handoff not durable: %#v %v",messages,err)}
 if err:=s.Handoff(msg);err==nil {t.Fatal("duplicate handoff accepted")}
}

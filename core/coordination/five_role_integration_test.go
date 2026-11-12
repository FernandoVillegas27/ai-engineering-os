package coordination

import (
 "fmt"
 "path/filepath"
 "testing"
)

// Tests the deterministic five-role handoff and restart path, not AI execution.
func TestDurableFiveRoleWorkflowAcrossRestart(t *testing.T) {
 roles:=[]Role{Architect,Backend,Frontend,QA,Reviewer}
 ids:=[]string{"architecture","backend","frontend","qa","review"}
 tasks:=make([]Task,len(ids))
 for i:=range ids {
  tasks[i]=Task{ID:ids[i],Role:roles[i],State:Queued}
  if i>0 {tasks[i].DependsOn=[]string{ids[i-1]}}
 }
 dir:=t.TempDir()
 statePath:=filepath.Join(dir,"state.jsonl")
 messagesPath:=filepath.Join(dir,"messages.jsonl")
 open:=func() *DurableSession {
  t.Helper()
  s,err:=OpenDurableSession("run-five",statePath,messagesPath,tasks)
  if err!=nil {t.Fatal(err)}
  return s
 }
 session:=open()
 for i,id:=range ids {
  if i==0 {
   if err:=session.Tasks.Transition(id,Ready);err!=nil {t.Fatal(err)}
  }
  for _,next:=range []State{Running,Completed} {
   if err:=session.Tasks.Transition(id,next);err!=nil {t.Fatalf("%s -> %s: %v",id,next,err)}
  }
  if i+1<len(ids) {
   recipient:=ids[i+1]
   if err:=session.Tasks.Transition(recipient,Ready);err!=nil {t.Fatal(err)}
   msg:=Message{ID:fmt.Sprintf("handoff-%d",i),RunID:"run-five",FromTask:id,ToTask:recipient,Kind:Handoff,Body:"verified task output"}
   if err:=session.Send(msg);err!=nil {t.Fatal(err)}
   session=open()
   snapshot:=session.Tasks.Snapshot()
   if snapshot[i].State!=Completed || snapshot[i+1].State!=Ready {t.Fatalf("recovered state: %#v",snapshot)}
   replayed,err:=session.Messages.Replay()
   if err!=nil {t.Fatal(err)}
   if len(replayed)!=i+1 {t.Fatalf("replayed %d messages, want %d",len(replayed),i+1)}
  }
 }
 final:=open()
 snapshot:=final.Tasks.Snapshot()
 for _,task:=range snapshot {
  if task.State!=Completed {t.Fatalf("unfinished task: %#v",task)}
 }
}

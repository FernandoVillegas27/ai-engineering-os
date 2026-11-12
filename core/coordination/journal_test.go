package coordination

import (
 "os"
 "path/filepath"
 "testing"
)

func TestJournalAppendReplayAndRunIsolation(t *testing.T) {
 tasks:=[]Task{{ID:"architect",Role:Architect,State:Running},{ID:"backend",Role:Backend,State:Queued}}
 path:=filepath.Join(t.TempDir(),"messages.jsonl")
 j,err:=NewJournal(path,"run-1",tasks)
 if err!=nil {t.Fatal(err)}
 m:=Message{ID:"m1",RunID:"run-1",FromTask:"architect",ToTask:"backend",Kind:Handoff,Body:"Implement API",ArtifactIDs:[]string{"design"}}
 if err:=j.Append(m);err!=nil {t.Fatal(err)}
 if err:=j.Append(m);err==nil {t.Fatal("duplicate message accepted")}
 other:=m;other.ID="m2";other.RunID="run-2"
 if err:=j.Append(other);err==nil {t.Fatal("foreign run accepted")}
 restored,err:=NewJournal(path,"run-1",tasks)
 if err!=nil {t.Fatal(err)}
 messages,err:=restored.Replay()
 if err!=nil || len(messages)!=1 || messages[0].Body!="Implement API" {t.Fatalf("unexpected replay %#v %v",messages,err)}
 foreign,err:=NewJournal(path,"run-2",tasks)
 if err!=nil {t.Fatal(err)}
 if _,err:=foreign.Replay();err==nil {t.Fatal("foreign run journal replay accepted")}
}

func TestJournalRejectsCorruptionBeforeAppending(t *testing.T) {
 tasks:=[]Task{{ID:"a",Role:Architect,State:Running},{ID:"b",Role:Backend,State:Queued}}
 path:=filepath.Join(t.TempDir(),"messages.jsonl")
 if err:=os.WriteFile(path,[]byte("{invalid json}\n"),0600);err!=nil {t.Fatal(err)}
 j,err:=NewJournal(path,"run-1",tasks)
 if err!=nil {t.Fatal(err)}
 msg:=Message{ID:"m",RunID:"run-1",FromTask:"a",ToTask:"b",Kind:Question,Body:"Question"}
 if err:=j.Append(msg);err==nil {t.Fatal("corrupt journal must fail closed")}
 data,err:=os.ReadFile(path)
 if err!=nil || string(data)!="{invalid json}\n" {t.Fatalf("journal unexpectedly changed %q %v",data,err)}
}

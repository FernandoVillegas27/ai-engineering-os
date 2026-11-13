package coordination

import (
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestOwnedDurableSessionExclusiveAcrossOpenAndReplay(t *testing.T){
 dir:=t.TempDir()
 lease:=filepath.Join(dir,"run.lock")
 states:=filepath.Join(dir,"states.jsonl")
 messages:=filepath.Join(dir,"messages.jsonl")
 tasks:=[]Task{{ID:"architect",Role:Architect,State:Queued}}
 first,err:=OpenOwnedDurableSession("run",lease,states,messages,tasks)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenOwnedDurableSession("run",lease,states,messages,tasks);!errors.Is(err,os.ErrExist){t.Fatalf("concurrent writer not rejected: %v",err)}
 if err:=first.Tasks.Transition("architect",Ready);err!=nil{t.Fatal(err)}
 if err:=first.Close();err!=nil{t.Fatal(err)}
 second,err:=OpenOwnedDurableSession("run",lease,states,messages,tasks)
 if err!=nil{t.Fatal(err)}
 if second.Tasks.Snapshot()[0].State!=Ready{t.Fatal("replay lost task state")}
 if err:=second.Close();err!=nil{t.Fatal(err)}
}

func TestOwnedDurableSessionReleasesLeaseAfterInvalidJournal(t *testing.T){
 dir:=t.TempDir()
 lease:=filepath.Join(dir,"run.lock")
 states:=filepath.Join(dir,"states.jsonl")
 messages:=filepath.Join(dir,"messages.jsonl")
 tasks:=[]Task{{ID:"architect",Role:Architect,State:Queued}}
 if err:=os.WriteFile(states,[]byte("{bad json}\n"),0600);err!=nil{t.Fatal(err)}
 if _,err:=OpenOwnedDurableSession("run",lease,states,messages,tasks);err==nil{t.Fatal("corrupt journal accepted")}
 if _,err:=os.Stat(lease);!errors.Is(err,os.ErrNotExist){t.Fatalf("failed open left lease marker: %v",err)}
}

func TestOwnedDurableSessionRejectsLeaseJournalAlias(t *testing.T){
 dir:=t.TempDir()
 states:=filepath.Join(dir,"states.jsonl")
 messages:=filepath.Join(dir,"messages.jsonl")
 tasks:=[]Task{{ID:"architect",Role:Architect,State:Queued}}
 if _,err:=OpenOwnedDurableSession("run",states,states,messages,tasks);err==nil{t.Fatal("lease and journal alias accepted")}
}

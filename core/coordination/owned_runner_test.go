package coordination

import (
 "context"
 "path/filepath"
 "testing"
)

func TestRunOwnedUntilIdleRequiresLeaseAndReplays(t *testing.T) {
 dir:=t.TempDir()
 lease:=filepath.Join(dir,"run.lock")
 tasksPath:=filepath.Join(dir,"tasks.jsonl")
 messagesPath:=filepath.Join(dir,"messages.jsonl")
 tasks:=[]Task{{ID:"plan",Role:Architect,State:Queued},{ID:"qa",Role:QA,State:Queued,DependsOn:[]string{"plan"}}}
 if err:=RunOwnedUntilIdle(context.Background(),nil,&recordingExecutor{});err==nil {t.Fatal("unowned run accepted")}
 owned,err:=OpenOwnedDurableSession("run",lease,tasksPath,messagesPath,tasks)
 if err!=nil {t.Fatal(err)}
 second,err:=OpenOwnedDurableSession("run",lease,tasksPath,messagesPath,tasks)
 if err==nil { _=second.Close();t.Fatal("second writer acquired lease") }
 executor:=&recordingExecutor{}
 if err:=RunOwnedUntilIdle(context.Background(),owned,executor);err!=nil {t.Fatal(err)}
 if len(executor.called)!=2 || executor.called[0]!="plan" || executor.called[1]!="qa" {t.Fatalf("wrong execution order: %v",executor.called)}
 if err:=owned.Close();err!=nil {t.Fatal(err)}
 if err:=RunOwnedUntilIdle(context.Background(),owned,executor);err==nil {t.Fatal("released lease accepted")}
 reopened,err:=OpenOwnedDurableSession("run",lease,tasksPath,messagesPath,tasks)
 if err!=nil {t.Fatal(err)}
 defer reopened.Close()
 if err:=RunOwnedUntilIdle(context.Background(),reopened,executor);err!=nil {t.Fatal(err)}
 if len(executor.called)!=2 {t.Fatal("completed work executed again")}
}

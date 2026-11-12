package coordination

import (
 "context"
 "path/filepath"
 "testing"
)

func TestRecoverInterruptedQuarantinesWithoutRetry(t *testing.T) {
 tasks:=[]Task{{ID:"architect",Role:Architect,State:Queued},{ID:"backend",Role:Backend,State:Queued,DependsOn:[]string{"architect"}}}
 path:=filepath.Join(t.TempDir(),"tasks.jsonl")
 scheduler,err:=NewDurableScheduler("run",path,tasks)
 if err!=nil {t.Fatal(err)}
 if err:=scheduler.Transition("architect",Ready);err!=nil {t.Fatal(err)}
 if err:=scheduler.Transition("architect",Running);err!=nil {t.Fatal(err)}
 restarted,err:=NewDurableScheduler("run",path,tasks)
 if err!=nil {t.Fatal(err)}
 quarantined,err:=RecoverInterrupted(restarted)
 if err!=nil {t.Fatal(err)}
 if len(quarantined)!=1 || quarantined[0]!="architect" {t.Fatalf("wrong quarantine: %v",quarantined)}
 again,err:=NewDurableScheduler("run",path,tasks)
 if err!=nil {t.Fatal(err)}
 if again.Snapshot()[0].State!=Blocked {t.Fatal("quarantine was not persisted")}
 exec:=&recordingExecutor{}
 if err:=RunReady(context.Background(),again,exec);err!=nil {t.Fatal(err)}
 if len(exec.called)!=0 {t.Fatal("blocked task was automatically retried")}
 if err:=again.Transition("architect",Ready);err!=nil {t.Fatal(err)}
 if err:=RunReady(context.Background(),again,exec);err!=nil {t.Fatal(err)}
 if len(exec.called)!=1 || exec.called[0]!="architect" {t.Fatalf("explicit recovery failed: %v",exec.called)}
}

func TestRecoverInterruptedIsIdempotent(t *testing.T) {
 tasks:=[]Task{{ID:"architect",Role:Architect,State:Queued}}
 scheduler,err:=NewDurableScheduler("run",filepath.Join(t.TempDir(),"tasks.jsonl"),tasks)
 if err!=nil {t.Fatal(err)}
 first,err:=RecoverInterrupted(scheduler)
 if err!=nil || len(first)!=0 {t.Fatalf("unexpected recovery: %v %v",first,err)}
 if err:=scheduler.Transition("architect",Ready);err!=nil {t.Fatal(err)}
 if err:=scheduler.Transition("architect",Running);err!=nil {t.Fatal(err)}
 if _,err:=RecoverInterrupted(scheduler);err!=nil {t.Fatal(err)}
 second,err:=RecoverInterrupted(scheduler)
 if err!=nil || len(second)!=0 {t.Fatalf("duplicate recovery: %v %v",second,err)}
}

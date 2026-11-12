package coordination

import (
 "os"
 "path/filepath"
 "testing"
)

func TestDurableSchedulerReplayAndFailClosed(t *testing.T) {
 tasks:=[]Task{{ID:"architect",Role:Architect,State:Queued},{ID:"backend",Role:Backend,State:Queued,DependsOn:[]string{"architect"}}}
 path:=filepath.Join(t.TempDir(),"tasks.jsonl")
 d,err:=NewDurableScheduler("run1",path,tasks)
 if err!=nil {t.Fatal(err)}
 if err:=d.Transition("backend",Ready);err==nil {t.Fatal("dependency bypass accepted")}
 for _,next:=range []State{Ready,Running,Completed} {
  if err:=d.Transition("architect",next);err!=nil {t.Fatal(err)}
 }
 if err:=d.Transition("backend",Ready);err!=nil {t.Fatal(err)}
 restored,err:=NewDurableScheduler("run1",path,tasks)
 if err!=nil {t.Fatal(err)}
 snapshot:=restored.Snapshot()
 if snapshot[0].State!=Completed || snapshot[1].State!=Ready {t.Fatalf("incorrect replay: %#v",snapshot)}
 if _,err:=NewDurableScheduler("other-run",path,tasks);err==nil {t.Fatal("foreign run accepted")}
 if err:=os.WriteFile(path,[]byte("{invalid}\n"),0600);err!=nil {t.Fatal(err)}
 if _,err:=NewDurableScheduler("run1",path,tasks);err==nil {t.Fatal("corrupt journal accepted")}
}

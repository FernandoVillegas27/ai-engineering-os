package coordination

import (
 "context"
 "path/filepath"
 "testing"
)

func TestInstructionExecutorFailsClosedWithoutInstructions(t *testing.T) {
 dir:=t.TempDir()
 executor:=&InstructionExecutor{Command:&CommandExecutor{Binary:"go",Workdir:dir},Instructions:map[string]string{}}
 task:=Task{ID:"plan",Role:Architect,State:Queued}
 if err:=executor.Execute(context.Background(),task);err==nil {t.Fatal("missing instructions accepted")}
 executor.Instructions["plan"]="  "
 if err:=executor.Execute(context.Background(),task);err==nil {t.Fatal("blank instructions accepted")}
 if err:=(&CommandExecutor{Binary:"go",Workdir:dir}).Execute(context.Background(),task);err==nil {t.Fatal("placeholder CLI execution accepted")}
}

func TestMissingInstructionsDoNotCompleteDurableWork(t *testing.T) {
 tasks:=[]Task{{ID:"plan",Role:Architect,State:Queued}}
 path:=filepath.Join(t.TempDir(),"tasks.jsonl")
 scheduler,err:=NewDurableScheduler("run",path,tasks)
 if err!=nil {t.Fatal(err)}
 executor:=&InstructionExecutor{Command:&CommandExecutor{Binary:"go",Workdir:t.TempDir()}}
 if err:=RunUntilIdle(context.Background(),scheduler,executor);err==nil {t.Fatal("empty task executed")}
 recovered,err:=NewDurableScheduler("run",path,tasks)
 if err!=nil {t.Fatal(err)}
 if recovered.Snapshot()[0].State!=Failed {t.Fatalf("missing instructions incorrectly completed: %v",recovered.Snapshot()[0].State)}
}

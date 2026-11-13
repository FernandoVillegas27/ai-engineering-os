package coordination

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestDurableResultExecutorPersistsAndReplays(t *testing.T){
 if os.PathSeparator=='\\'{t.Skip("POSIX helper only")}
 dir:=t.TempDir()
 script:=filepath.Join(dir,"agent.sh")
 if err:=os.WriteFile(script,[]byte("#!/bin/sh\ncat\n"),0700);err!=nil{t.Fatal(err)}
 results:=filepath.Join(dir,"results")
 if err:=os.Mkdir(results,0700);err!=nil{t.Fatal(err)}
 task:=Task{ID:"../plan",Role:Architect,State:Ready}
 executor:=&DurableResultExecutor{Agent:&ResultExecutor{
  Command:&CommandExecutor{Binary:script,Workdir:dir},
  Instructions:map[string]string{task.ID:"design API"},
  MaxOutputBytes:4096,
 },Directory:results}
 if err:=executor.Execute(context.Background(),task);err!=nil{t.Fatal(err)}
 got,err:=executor.Read(task.ID)
 if err!=nil{t.Fatal(err)}
 if got.Role!=Architect||!strings.Contains(got.Output,"design API"){t.Fatalf("unexpected result: %#v",got)}
 if _,err:=os.Stat(resultPath(results,task.ID));err!=nil{t.Fatal(err)}
 if err:=executor.Execute(context.Background(),task);err==nil{t.Fatal("duplicate durable result accepted")}
 restarted:=&DurableResultExecutor{Directory:results}
 replay,err:=restarted.Read(task.ID)
 if err!=nil||replay.Output!=got.Output{t.Fatalf("replay failed: %v",err)}
 if _,err:=restarted.Read("other");!errors.Is(err,os.ErrNotExist){t.Fatalf("unknown task: %v",err)}
}

func TestDurableResultExecutorRejectsInvalidDirectory(t *testing.T){
 e:=&DurableResultExecutor{Agent:&ResultExecutor{},Directory:filepath.Join(t.TempDir(),"missing")}
 if err:=e.Execute(context.Background(),Task{ID:"x",Role:Architect});err==nil{t.Fatal("missing directory accepted")}
}

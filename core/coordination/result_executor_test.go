package coordination

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestResultExecutorRequiresInstructionsAndLimit(t *testing.T){
 task:=Task{ID:"plan",Role:Architect,State:Ready}
 e:=&ResultExecutor{Command:&CommandExecutor{Binary:"go",Workdir:t.TempDir()},MaxOutputBytes:1024}
 if err:=e.Execute(context.Background(),task);err==nil{t.Fatal("missing instructions accepted")}
 e.Instructions=map[string]string{"plan":"write a plan"}
 e.MaxOutputBytes=0
 if err:=e.Execute(context.Background(),task);err==nil{t.Fatal("unbounded output accepted")}
}

func TestResultExecutorCapturesAndRejectsDuplicate(t *testing.T){
 if os.PathSeparator=='\\'{t.Skip("POSIX helper only")}
 dir:=t.TempDir()
 script:=filepath.Join(dir,"agent.sh")
 if err:=os.WriteFile(script,[]byte("#!/bin/sh\ncat\n"),0700);err!=nil{t.Fatal(err)}
 e:=&ResultExecutor{Command:&CommandExecutor{Binary:script,Workdir:dir},Instructions:map[string]string{"plan":"design a service"},MaxOutputBytes:4096}
 task:=Task{ID:"plan",Role:Architect,State:Ready}
 if err:=e.Execute(context.Background(),task);err!=nil{t.Fatal(err)}
 result,ok:=e.Result("plan")
 if !ok||result.Role!=Architect||!strings.Contains(result.Output,"design a service"){t.Fatalf("missing agent evidence: %#v",result)}
 if err:=e.Execute(context.Background(),task);err==nil{t.Fatal("duplicate result accepted")}
}

func TestResultExecutorRejectsEmptyAndOversizedOutput(t *testing.T){
 if os.PathSeparator=='\\'{t.Skip("POSIX helper only")}
 dir:=t.TempDir()
 for _,tc:=range []struct{name,script string;limit int}{
  {"empty","#!/bin/sh\nexit 0\n",1024},
  {"oversize","#!/bin/sh\nprintf 'this output is too large'\n",4},
 }{
  t.Run(tc.name,func(t *testing.T){
   path:=filepath.Join(dir,tc.name+".sh")
   if err:=os.WriteFile(path,[]byte(tc.script),0700);err!=nil{t.Fatal(err)}
   e:=&ResultExecutor{Command:&CommandExecutor{Binary:path,Workdir:dir},Instructions:map[string]string{"plan":"write"},MaxOutputBytes:tc.limit}
   if err:=e.Execute(context.Background(),Task{ID:"plan",Role:Architect,State:Ready});err==nil{t.Fatal("invalid output accepted")}
   if _,ok:=e.Result("plan");ok{t.Fatal("invalid result persisted")}
  })
 }
}

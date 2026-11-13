package coordination

import (
 "bytes"
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestCommandExecutorRequiresExplicitConfiguration(t *testing.T) {
 task:=Task{ID:"plan",Role:Architect,State:Ready}
 cases:=[]CommandExecutor{{},{Binary:"go"},{Binary:"go",Workdir:filepath.Join(t.TempDir(),"missing")}}
 for _,e:=range cases {
  if err:=(&InstructionExecutor{Command:&e,Instructions:map[string]string{"plan":"produce a plan"}}).Execute(context.Background(),task);err==nil {t.Fatal("invalid executor configuration accepted")}
 }
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 e:=&CommandExecutor{Binary:"go",Workdir:t.TempDir()}
 if err:=(&InstructionExecutor{Command:e,Instructions:map[string]string{"plan":"produce a plan"}}).Execute(ctx,task);!errors.Is(err,context.Canceled) {t.Fatalf("cancelled execution: %v",err)}
}

func TestCommandExecutorUsesStdinWithoutShellInterpolation(t *testing.T) {
 dir:=t.TempDir()
 script:=filepath.Join(dir,"agent.sh")
 // This test runs only on POSIX systems; Windows tests use the configuration
 // and cancellation cases above, without depending on a Unix shell.
 if os.PathSeparator=='\\' {t.Skip("POSIX helper only")}
 if err:=os.WriteFile(script,[]byte("#!/bin/sh\ncat\n"),0700);err!=nil {t.Fatal(err)}
 var out bytes.Buffer
 e:=&CommandExecutor{Binary:script,Workdir:dir,Output:&out}
 task:=Task{ID:"plan; echo UNSAFE",Role:Architect,State:Ready}
 if err:=(&InstructionExecutor{Command:e,Instructions:map[string]string{task.ID:"write a design"}}).Execute(context.Background(),task);err!=nil {t.Fatal(err)}
 if !strings.Contains(out.String(),"Task ID: plan; echo UNSAFE") || !strings.Contains(out.String(),"write a design") {t.Fatalf("task input was not preserved as data: %q",out.String())}
}

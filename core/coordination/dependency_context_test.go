package coordination

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestBuildDependentInstructionsUsesOnlyCompletedPersistedOutputs(t *testing.T){
 dir:=t.TempDir()
 predecessor:=Task{ID:"architect",Role:Architect,State:Completed}
 task:=Task{ID:"backend",Role:Backend,State:Queued,DependsOn:[]string{"architect"}}
 store:=&DurableResultExecutor{Directory:dir}
 data:=`{"TaskID":"architect","Role":"architect","Output":"Use REST endpoints"}`
 if err:=os.WriteFile(filepath.Join(dir,"617263686974656374.json"),[]byte(data),0600);err!=nil{t.Fatal(err)}
 prompt,err:=BuildDependentInstructions(task,map[string]string{"backend":"Implement backend"},[]Task{predecessor,task},store)
 if err!=nil || !strings.Contains(prompt,"Implement backend") || !strings.Contains(prompt,"Use REST endpoints"){
  t.Fatalf("missing dependency context: %q %v",prompt,err)
 }
 predecessor.State=Running
 if _,err:=BuildDependentInstructions(task,map[string]string{"backend":"Implement backend"},[]Task{predecessor,task},store);err==nil {
  t.Fatal("running dependency accepted")
 }
 predecessor.State=Completed
 if _,err:=BuildDependentInstructions(task,map[string]string{"backend":" "},[]Task{predecessor,task},store);err==nil{
  t.Fatal("empty task instruction accepted")
 }
 if _,err:=BuildDependentInstructions(task,map[string]string{"backend":"Implement backend"},[]Task{task},store);err==nil{
  t.Fatal("missing predecessor accepted")
 }
}

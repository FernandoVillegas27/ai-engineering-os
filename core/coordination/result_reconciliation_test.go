package coordination

import (
 "os"
 "path/filepath"
 "testing"
)

func TestInspectDurableResultsFlagsCrashWindowWithoutMutation(t *testing.T) {
 dir:=t.TempDir()
 scheduler,err:=NewDurableScheduler("run",filepath.Join(dir,"tasks.jsonl"),[]Task{{ID:"plan",Role:Architect,State:Queued}})
 if err!=nil {t.Fatal(err)}
 for _,state:=range []State{Ready,Running} {
  if err:=scheduler.Transition("plan",state);err!=nil {t.Fatal(err)}
 }
 results:=filepath.Join(dir,"results")
 if err:=os.Mkdir(results,0700);err!=nil {t.Fatal(err)}
 file:=resultPath(results,"plan")
 if err:=os.WriteFile(file,[]byte(`{"TaskID":"plan","Role":"architect","Output":"plan output"}`),0600);err!=nil {t.Fatal(err)}
 store:=&DurableResultExecutor{Directory:results}
 report,err:=InspectDurableResults(scheduler,store)
 if err!=nil {t.Fatal(err)}
 if len(report)!=1 || !report[0].HasResult || !report[0].RequiresReview || report[0].State!=Running {
  t.Fatalf("expected reviewable crash window, got %#v",report)
 }
 if scheduler.Snapshot()[0].State!=Running {t.Fatal("inspection mutated task state")}
 if err:=scheduler.Transition("plan",Completed);err!=nil {t.Fatal(err)}
 report,err=InspectDurableResults(scheduler,store)
 if err!=nil || report[0].RequiresReview {t.Fatalf("consistent completion: %#v %v",report,err)}
}

func TestInspectDurableResultsRejectsCorruption(t *testing.T) {
 dir:=t.TempDir()
 scheduler,err:=NewDurableScheduler("run",filepath.Join(dir,"tasks.jsonl"),[]Task{{ID:"plan",Role:Architect,State:Queued}})
 if err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(resultPath(dir,"plan"),[]byte("not json"),0600);err!=nil {t.Fatal(err)}
 if _,err:=InspectDurableResults(scheduler,&DurableResultExecutor{Directory:dir});err==nil {
  t.Fatal("corrupt result was treated as missing")
 }
}

func TestInspectDurableResultsDetectsMissingCompletedOutput(t *testing.T) {
 dir:=t.TempDir()
 scheduler,err:=NewDurableScheduler("run",filepath.Join(dir,"tasks.jsonl"),[]Task{{ID:"plan",Role:Architect,State:Queued}})
 if err!=nil {t.Fatal(err)}
 for _,state:=range []State{Ready,Running,Completed} {
  if err:=scheduler.Transition("plan",state);err!=nil {t.Fatal(err)}
 }
 report,err:=InspectDurableResults(scheduler,&DurableResultExecutor{Directory:dir})
 if err!=nil || !report[0].RequiresReview || report[0].HasResult {
  t.Fatalf("missing completed output: %#v %v",report,err)
 }
}

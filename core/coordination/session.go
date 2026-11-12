package coordination

import (
 "errors"
 "fmt"
 "sync"
)

// Session combines a task scheduler and run-scoped durable communication.
// It is a coordination boundary, not an agent runtime or permission grant.
type Session struct {
 mu sync.Mutex
 RunID string
 Scheduler *Scheduler
 Messages *Journal
}

func NewSession(runID, journalPath string, tasks []Task) (*Session,error) {
 if runID=="" {return nil,errors.New("run ID required")}
 scheduler,err:=NewScheduler(tasks)
 if err!=nil {return nil,err}
 journal,err:=NewJournal(journalPath,runID,tasks)
 if err!=nil {return nil,err}
 if _,err=journal.Replay();err!=nil {return nil,err}
 return &Session{RunID:runID,Scheduler:scheduler,Messages:journal},nil
}

// Handoff records a message before moving the recipient from queued to ready.
// A handoff cannot bypass the scheduler's dependency gate.
func (s *Session) Handoff(msg Message) error {
 s.mu.Lock()
 defer s.mu.Unlock()
 if msg.RunID!=s.RunID || msg.Kind!=Handoff {return errors.New("handoff must match session and message kind")}
 tasks:=s.Scheduler.Snapshot()
 if err:=ValidateMessage(msg,tasks);err!=nil {return err}
 var from,to *Task
 for i:=range tasks {
  if tasks[i].ID==msg.FromTask {from=&tasks[i]}
  if tasks[i].ID==msg.ToTask {to=&tasks[i]}
 }
 if from==nil || to==nil || from.State!=Completed || to.State!=Queued {
  return errors.New("handoff requires completed sender and queued recipient")
 }
 ready,err:=s.Scheduler.Ready()
 if err!=nil {return err}
 eligible:=false
 for _,task:=range ready {if task.ID==to.ID {eligible=true}}
 if !eligible {return fmt.Errorf("recipient %q dependencies are not complete",to.ID)}
 if err:=s.Messages.Append(msg);err!=nil {return err}
 return s.Scheduler.Transition(to.ID,Ready)
}

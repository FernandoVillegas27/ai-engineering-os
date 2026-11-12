package coordination

import (
 "errors"
 "fmt"
 "sync"
)

// DurableSession composes replayable task states and run-scoped messages.
// The enclosing run store must guarantee exclusive ownership of both paths.
// Cross-journal atomic transactions are not provided by this adapter.
type DurableSession struct {
 mu sync.Mutex
 RunID string
 Tasks *DurableScheduler
 Messages *Journal
}

func OpenDurableSession(runID, taskJournalPath, messageJournalPath string, tasks []Task) (*DurableSession,error) {
 if taskJournalPath==messageJournalPath {return nil,errors.New("task and message journals must be separate")}
 scheduler,err:=NewDurableScheduler(runID,taskJournalPath,tasks)
 if err!=nil {return nil,err}
 messages,err:=NewJournal(messageJournalPath,runID,tasks)
 if err!=nil {return nil,err}
 if _,err=messages.Replay();err!=nil {return nil,err}
 return &DurableSession{RunID:runID,Tasks:scheduler,Messages:messages},nil
}

// Send validates messages against the current run task graph and records them.
// A handoff is only accepted from a completed sender to a ready recipient.
// Task state transitions must be persisted separately through Tasks.Transition.
func (s *DurableSession) Send(msg Message) error {
 s.mu.Lock()
 defer s.mu.Unlock()
 if msg.RunID!=s.RunID {return errors.New("foreign run message")}
 tasks:=s.Tasks.Snapshot()
 if err:=ValidateMessage(msg,tasks);err!=nil {return err}
 if msg.Kind==Handoff {
  var from,to *Task
  for i:=range tasks {
   if tasks[i].ID==msg.FromTask {from=&tasks[i]}
   if tasks[i].ID==msg.ToTask {to=&tasks[i]}
  }
  if from==nil || to==nil || from.State!=Completed || to.State!=Ready {
   return fmt.Errorf("handoff requires completed sender and ready recipient")
  }
 }
 return s.Messages.Append(msg)
}

package coordination

import (
 "bufio"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sync"
)

// Journal provides append-only, replayable inter-agent communication for one run.
// Callers must serialize access to a journal path across processes; its mutex only
// coordinates concurrent writers within this instance.
type Journal struct {
 mu sync.Mutex
 path string
 runID string
 tasks []Task
}

func NewJournal(path, runID string, tasks []Task) (*Journal,error) {
 if path == "" || runID == "" { return nil,errors.New("journal path and run ID are required") }
 if err:=ValidateGraph(tasks);err!=nil {return nil,err}
 if len(tasks)==0 {return nil,errors.New("journal requires tasks")}
 copyTasks:=make([]Task,len(tasks))
 for i,t:=range tasks {copyTasks[i]=t;copyTasks[i].DependsOn=append([]string(nil),t.DependsOn...)}
 return &Journal{path:path,runID:runID,tasks:copyTasks},nil
}

func (j *Journal) Replay() ([]Message,error) {
 j.mu.Lock()
 defer j.mu.Unlock()
 return j.replay()
}

func (j *Journal) replay() ([]Message,error) {
 f,err:=os.Open(j.path)
 if errors.Is(err,os.ErrNotExist) {return []Message{},nil}
 if err!=nil {return nil,err}
 defer f.Close()
 scanner:=bufio.NewScanner(f)
 scanner.Buffer(make([]byte,64*1024),4*1024*1024)
 seen:=map[string]bool{}
 messages:=[]Message{}
 for scanner.Scan() {
  var msg Message
  if err:=json.Unmarshal(scanner.Bytes(),&msg);err!=nil {return nil,fmt.Errorf("corrupt journal entry %d: %w",len(messages)+1,err)}
  if msg.RunID!=j.runID {return nil,fmt.Errorf("journal contains foreign run %q",msg.RunID)}
  if err:=ValidateMessage(msg,j.tasks);err!=nil {return nil,fmt.Errorf("invalid journal message: %w",err)}
  if seen[msg.ID] {return nil,fmt.Errorf("duplicate journal message %q",msg.ID)}
  seen[msg.ID]=true
  messages=append(messages,msg)
 }
 if err:=scanner.Err();err!=nil {return nil,err}
 return messages,nil
}

func (j *Journal) Append(msg Message) error {
 j.mu.Lock()
 defer j.mu.Unlock()
 if msg.RunID!=j.runID {return errors.New("message belongs to another run")}
 if err:=ValidateMessage(msg,j.tasks);err!=nil {return err}
 previous,err:=j.replay()
 if err!=nil {return err}
 for _,old:=range previous {if old.ID==msg.ID {return fmt.Errorf("duplicate message %q",msg.ID)}}
 // Parent must already exist: creation and access policy belong to the
 // enclosing run store. Never silently create directories.
 if _,err:=os.Stat(filepath.Dir(j.path));err!=nil {return err}
 data,err:=json.Marshal(msg)
 if err!=nil {return err}
 f,err:=os.OpenFile(j.path,os.O_WRONLY|os.O_APPEND|os.O_CREATE,0600)
 if err!=nil {return err}
 if _,err=f.Write(append(data,'\n'));err!=nil {f.Close();return err}
 if err=f.Sync();err!=nil {f.Close();return err}
 return f.Close()
}

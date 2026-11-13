package coordination

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sync"
)

// RunLease is an exclusive, fail-closed filesystem ownership marker for a run.
// Every process accessing the same journals must acquire the same lease path.
// A crash deliberately leaves the marker in place: recovery requires explicit
// operator reconciliation before the marker is removed. No TTL-based stealing.
type RunLease struct {
 mu sync.Mutex
 path string
 file *os.File
 closed bool
}

// AcquireRunLease creates a marker atomically. Existing markers always block,
// including stale markers from crashed processes. Do not place the marker on a
// different filesystem or shared path than the journal ownership domain.
func AcquireRunLease(path string) (*RunLease,error) {
 if path=="" {return nil,errors.New("run lease path required")}
 abs,err:=filepath.Abs(path)
 if err!=nil{return nil,err}
 f,err:=os.OpenFile(abs,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil {
  if errors.Is(err,os.ErrExist){return nil,fmt.Errorf("run lease already held or requires recovery: %w",err)}
  return nil,err
 }
 return &RunLease{path:abs,file:f},nil
}

// Close releases the marker. If the file was replaced, it is not removed:
// callers must not delete ownership belonging to another process.
func (l *RunLease) Close() error {
 if l==nil{return errors.New("nil run lease")}
 l.mu.Lock()
 defer l.mu.Unlock()
 if l.closed{return nil}
 current,err:=os.Stat(l.path)
 if err!=nil{return fmt.Errorf("lease marker unavailable: %w",err)}
 owned,err:=l.file.Stat()
 if err!=nil{return err}
 if !os.SameFile(current,owned){return errors.New("lease marker replaced; refusing to release foreign ownership")}
 if err:=os.Remove(l.path);err!=nil{return err}
 l.closed=true
 return l.file.Close()
}

package coordination

import (
 "errors"
 "fmt"
)

// OwnedDurableSession binds both journals to a single acquired run lease.
// Only use its Tasks and Messages while ownership is held. All processes
// operating on these journals must follow the same lease convention.
type OwnedDurableSession struct {
 *DurableSession
 lease *RunLease
}

// OpenOwnedDurableSession acquires exclusive ownership before replaying either
// journal, and releases the lease if journal validation fails.
func OpenOwnedDurableSession(runID,leasePath,taskJournalPath,messageJournalPath string,tasks []Task)(*OwnedDurableSession,error){
 if leasePath==taskJournalPath || leasePath==messageJournalPath {
  return nil,errors.New("lease path must differ from journal paths")
 }
 lease,err:=AcquireRunLease(leasePath)
 if err!=nil{return nil,err}
 if err:=distinctJournalPaths(lease.path,taskJournalPath);err!=nil {
  _=lease.Close()
  return nil,fmt.Errorf("lease and task journal path conflict: %w",err)
 }
 if err:=distinctJournalPaths(lease.path,messageJournalPath);err!=nil {
  _=lease.Close()
  return nil,fmt.Errorf("lease and message journal path conflict: %w",err)
 }
 session,err:=OpenDurableSession(runID,taskJournalPath,messageJournalPath,tasks)
 if err!=nil {
  _=lease.Close()
  return nil,err
 }
 return &OwnedDurableSession{DurableSession:session,lease:lease},nil
}

// Close releases exclusive ownership after all agent work has stopped.
func(s *OwnedDurableSession) Close()error{
 if s==nil{return errors.New("nil owned session")}
 return s.lease.Close()
}

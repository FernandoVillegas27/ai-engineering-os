package coordination

import (
 "context"
 "errors"
)

// RunOwnedUntilIdle is the safe entry point for executing a durable session.
// The session must have been opened through OpenOwnedDurableSession so that
// another cooperating process cannot mutate the same journals concurrently.
// It never grants agent tools, approvals, or repository merge permissions.
func RunOwnedUntilIdle(ctx context.Context, session *OwnedDurableSession, executor Executor) error {
 if session==nil || session.lease==nil || session.DurableSession==nil {
  return errors.New("owned durable session required")
 }
 session.lease.mu.Lock()
 defer session.lease.mu.Unlock()
 if session.lease.closed {return errors.New("run lease already released")}
 if session.Tasks==nil {return errors.New("owned task scheduler required")}
 return RunUntilIdle(ctx,session.Tasks,executor)
}

package coordination

import (
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestRunLeaseRejectsConcurrentAndStaleOwners(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"run.lock")
 first,err:=AcquireRunLease(path)
 if err!=nil{t.Fatal(err)}
 if _,err:=AcquireRunLease(path);!errors.Is(err,os.ErrExist){t.Fatalf("expected exclusive lease collision, got %v",err)}
 if err:=first.Close();err!=nil{t.Fatal(err)}
 second,err:=AcquireRunLease(path)
 if err!=nil{t.Fatal(err)}
 if err:=second.Close();err!=nil{t.Fatal(err)}
 if err:=second.Close();err!=nil{t.Fatal(err)}
 // A crashed owner leaves a marker; reopening must fail closed.
 if err:=os.WriteFile(path,[]byte("unreconciled"),0600);err!=nil{t.Fatal(err)}
 if _,err:=AcquireRunLease(path);!errors.Is(err,os.ErrExist){t.Fatalf("stale marker must not be stolen: %v",err)}
}

func TestRunLeaseNeverRemovesReplacedMarker(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"run.lock")
 lease,err:=AcquireRunLease(path)
 if err!=nil{t.Fatal(err)}
 if err:=os.Remove(path);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte("other-owner"),0600);err!=nil{t.Fatal(err)}
 if err:=lease.Close();err==nil{t.Fatal("replaced marker was released")}
 data,err:=os.ReadFile(path)
 if err!=nil{t.Fatal(err)}
 if string(data)!="other-owner"{t.Fatal("foreign marker was modified")}
 // Test cleanup only: the lease must not silently release ownership.
 if err:=lease.file.Close();err!=nil{t.Fatal(err)}
}

func TestRunLeaseRejectsEmptyPath(t *testing.T) {
 if _,err:=AcquireRunLease("");err==nil{t.Fatal("empty path accepted")}
}

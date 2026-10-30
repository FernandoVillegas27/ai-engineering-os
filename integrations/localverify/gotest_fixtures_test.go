package localverify

// JSONL fixtures live in Go source because the safe-view policy excludes .jsonl
// files, which would make file-based fixtures fail in every staged workspace.

// fixtureMixedFailures was captured from go test -json: failing test, failing subtest,
// build error, panic, and the package summary lines that follow failing tests.
const fixtureMixedFailures = `{"ImportPath":"probe/broken [probe/broken.test]","Action":"build-output","Output":"# probe/broken [probe/broken.test]\n"}
{"ImportPath":"probe/broken [probe/broken.test]","Action":"build-output","Output":"broken\\c_test.go:3:27: undefined: undefinedSymbol\n"}
{"ImportPath":"probe/broken [probe/broken.test]","Action":"build-fail"}
{"Action":"start","Package":"probe/bad"}
{"Action":"start","Package":"probe/boom"}
{"Action":"start","Package":"probe/broken"}
{"Action":"output","Package":"probe/broken","Output":"FAIL\tprobe/broken [build failed]\n"}
{"Action":"fail","Package":"probe/broken","Elapsed":0,"FailedBuild":"probe/broken [probe/broken.test]"}
{"Action":"start","Package":"probe/ok"}
{"Action":"run","Package":"probe/bad","Test":"TestOther"}
{"Action":"output","Package":"probe/bad","Test":"TestOther","Output":"=== RUN   TestOther\n"}
{"Action":"output","Package":"probe/bad","Test":"TestOther","Output":"    b_test.go:3: x\n"}
{"Action":"output","Package":"probe/bad","Test":"TestOther","Output":"--- FAIL: TestOther (0.00s)\n"}
{"Action":"fail","Package":"probe/bad","Test":"TestOther","Elapsed":0}
{"Action":"output","Package":"probe/bad","Output":"FAIL\n"}
{"Action":"output","Package":"probe/bad","Output":"FAIL\tprobe/bad\t0.152s\n"}
{"Action":"fail","Package":"probe/bad","Elapsed":0.154}
{"Action":"run","Package":"probe/boom","Test":"TestBoom"}
{"Action":"output","Package":"probe/boom","Test":"TestBoom","Output":"=== RUN   TestBoom\n"}
{"Action":"output","Package":"probe/boom","Test":"TestBoom","Output":"--- FAIL: TestBoom (0.00s)\n"}
{"Action":"run","Package":"probe/ok","Test":"TestPass"}
{"Action":"output","Package":"probe/ok","Test":"TestPass","Output":"=== RUN   TestPass\n"}
{"Action":"output","Package":"probe/ok","Test":"TestPass","Output":"--- PASS: TestPass (0.00s)\n"}
{"Action":"pass","Package":"probe/ok","Test":"TestPass","Elapsed":0}
{"Action":"run","Package":"probe/ok","Test":"TestFail"}
{"Action":"output","Package":"probe/ok","Test":"TestFail","Output":"=== RUN   TestFail\n"}
{"Action":"run","Package":"probe/ok","Test":"TestFail/sub"}
{"Action":"output","Package":"probe/ok","Test":"TestFail/sub","Output":"=== RUN   TestFail/sub\n"}
{"Action":"output","Package":"probe/ok","Test":"TestFail/sub","Output":"    a_test.go:4: nope\n"}
{"Action":"output","Package":"probe/ok","Test":"TestFail/sub","Output":"--- FAIL: TestFail/sub (0.00s)\n"}
{"Action":"fail","Package":"probe/ok","Test":"TestFail/sub","Elapsed":0}
{"Action":"output","Package":"probe/ok","Test":"TestFail","Output":"--- FAIL: TestFail (0.00s)\n"}
{"Action":"fail","Package":"probe/ok","Test":"TestFail","Elapsed":0}
{"Action":"output","Package":"probe/ok","Output":"FAIL\n"}
{"Action":"output","Package":"probe/boom","Test":"TestBoom","Output":"panic: kaboom [recovered, repanicked]\n"}
{"Action":"output","Package":"probe/boom","Test":"TestBoom","Output":"\n"}
{"Action":"output","Package":"probe/boom","Test":"TestBoom","Output":"goroutine 6 [running]:\n"}
{"Action":"output","Package":"probe/boom","Test":"TestBoom","Output":"testing.tRunner.func1.2({0x7ff7058b7400, 0x7ff7058fbfc0})\n"}
{"Action":"output","Package":"probe/boom","Test":"TestBoom","Output":"\tC:/Program Files/Go/src/testing/testing.go:1974 +0x239\n"}
{"Action":"output","Package":"probe/boom","Test":"TestBoom","Output":"probe/boom.TestBoom(0x3dcecf870400?)\n"}
{"Action":"output","Package":"probe/boom","Test":"TestBoom","Output":"\tC:/work/ws-a/boom/d_test.go:3 +0x25\n"}
{"Action":"fail","Package":"probe/boom","Test":"TestBoom","Elapsed":0}
{"Action":"output","Package":"probe/boom","Output":"FAIL\tprobe/boom\t0.195s\n"}
{"Action":"fail","Package":"probe/boom","Elapsed":0.196}
{"Action":"output","Package":"probe/ok","Output":"FAIL\tprobe/ok\t0.195s\n"}
{"Action":"fail","Package":"probe/ok","Elapsed":0.196}
`

// fixtureAllPass is a fully green stream.
const fixtureAllPass = `{"Action":"start","Package":"probe/ok"}
{"Action":"run","Package":"probe/ok","Test":"TestPass"}
{"Action":"output","Package":"probe/ok","Test":"TestPass","Output":"--- PASS: TestPass (0.00s)\n"}
{"Action":"pass","Package":"probe/ok","Test":"TestPass","Elapsed":0}
{"Action":"output","Package":"probe/ok","Output":"PASS\n"}
{"Action":"output","Package":"probe/ok","Output":"ok  \tprobe/ok\t0.120s\n"}
{"Action":"pass","Package":"probe/ok","Elapsed":0.12}
`

// fixturePackageFailureWithoutTests models a package that fails outside any test (for example TestMain).
const fixturePackageFailureWithoutTests = `{"Action":"start","Package":"probe/main"}
{"Action":"output","Package":"probe/main","Output":"setup failed: database unavailable\n"}
{"Action":"output","Package":"probe/main","Output":"FAIL\tprobe/main\t0.050s\n"}
{"Action":"fail","Package":"probe/main","Elapsed":0.05}
`

// fixtureDependentBuildFailure models a library compile error that breaks a dependent test package.
const fixtureDependentBuildFailure = `{"ImportPath":"probe/lib","Action":"build-output","Output":"# probe/lib\n"}
{"ImportPath":"probe/lib","Action":"build-output","Output":"lib\\lib.go:4:2: undefined: missing\n"}
{"ImportPath":"probe/lib","Action":"build-fail"}
{"Action":"start","Package":"probe/app"}
{"Action":"output","Package":"probe/app","Output":"FAIL\tprobe/app [build failed]\n"}
{"Action":"fail","Package":"probe/app","Elapsed":0,"FailedBuild":"probe/lib"}
`

// fixtureTruncatedStream is cut off while a test is still running, as after a timeout kill.
const fixtureTruncatedStream = `{"Action":"start","Package":"probe/slow"}
{"Action":"run","Package":"probe/slow","Test":"TestSlow"}
{"Action":"output","Package":"probe/slow","Test":"TestSlow","Output":"=== RUN   TestSlow\n"}
`

// fixtureWithGarbageLines mixes a failing test with lines that are not JSON events.
const fixtureWithGarbageLines = `{"Action":"start","Package":"probe/bad"}
warning: something printed outside the event stream
{"Action":"run","Package":"probe/bad","Test":"TestOther"}
{"Action":"output","Package":"probe/bad","Test":"TestOther","Output":"    b_test.go:3: x\n"}
{"Action":"fail","Package":"probe/bad","Test":"TestOther","Elapsed":0}
{"Action":"fail","Package":"probe/bad","Elapsed":0.1}
`

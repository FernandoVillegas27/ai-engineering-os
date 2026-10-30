package verification

// FailureClass describes what kind of unit failed. It is independent of the
// check-level FailureKind, which only separates code problems from infrastructure.
type FailureClass string

const (
	FailureClassTest    FailureClass = "test"
	FailureClassSubtest FailureClass = "subtest"
	FailureClassPanic   FailureClass = "panic"
	FailureClassPackage FailureClass = "package"
	FailureClassBuild   FailureClass = "build"
)

// Failure identifies one failing unit in a language-neutral way.
type Failure struct {
	// ID is stable across runs of the same code, for example "pkg#TestName/sub" or "pkg".
	ID    string
	Class FailureClass
	// Signature is a hash of the normalized failure output; see Signature.
	Signature string
}

// ParseConfidence states how much a verifier understood of a failed check's output.
type ParseConfidence string

const (
	// ParseNotAttempted means the verifier has no structured parser for this check.
	ParseNotAttempted ParseConfidence = ""
	// ParseComplete means every event was understood and the failures explain the failed exit.
	ParseComplete ParseConfidence = "complete"
	// ParsePartial means some output was not understood or the stream looks cut short.
	ParsePartial ParseConfidence = "partial"
	// ParseFailed means the output could not explain the failed exit.
	ParseFailed ParseConfidence = "failed"
)

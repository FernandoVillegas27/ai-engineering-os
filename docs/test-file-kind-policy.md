# Test File-Kind Policy

## Purpose

Agents with `tests: write` must not gain arbitrary source-code write access.

This policy provides a deliberately conservative repository-relative path classifier for a future QA-only mutation phase.

The policy prefers false negatives over permission escalation.

## Kinds

The classifier returns:

- `test`;
- `source`;
- `unknown`.

`unknown` is never accepted by a tests-only mutation scope.

## Test classification

A path is considered test-only when it has strong structural evidence.

Supported test directories include exact path components such as:

- `test`;
- `tests`;
- `__tests__`;
- `spec`;
- `specs`;
- `integration-test`;
- `integration-tests`.

Supported test filename conventions include:

- Go: `*_test.go`;
- Python: `test_*.py`, `*_test.py`;
- JavaScript/TypeScript: `*.test.*`, `*.spec.*` for JS/TS/JSX/TSX/MJS/CJS.

A Java class named `FooTest.java` is **not** assumed to be a test merely because of its class filename.

For example:

~~~text
src/main/java/com/acme/FooTest.java -> source
src/test/java/com/acme/FooTest.java -> test
~~~

This avoids granting test-only write permission based on naming conventions that are not sufficiently reliable outside a test source tree.

## Source classification

Known programming-language extensions outside an explicit test pattern are classified as `source`.

Examples include Go, Java, Kotlin, JavaScript, TypeScript, Python, C#, Ruby, PHP, Rust and C/C++ headers/sources.

Other file types remain `unknown`.

## Path safety

Classification requires a repository-relative path.

Absolute paths, traversal and NUL-containing paths are rejected.

## Tests-only validation

`ValidateTestsOnly` requires:

- at least one declared change;
- every declared path to classify as `test`.

A mixed proposal containing one test file and one source/unknown file is rejected as a whole.

## Intended QA execution order

This classifier does not put QA into the existing parallel source-writer stage.

The safe future order is:

~~~text
Researcher / Architect
  -> source-writer proposal + isolated stage
  -> source integration workspace
  -> QA tests-only proposal against integrated source result
  -> test-only isolated stage
  -> integrate QA test changes
  -> run-wide Quality Gate
  -> promotion candidate
~~~

This preserves the dependency graph and keeps `tests: write` narrower than `source_code: write`.

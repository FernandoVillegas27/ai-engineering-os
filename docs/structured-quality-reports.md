# Structured cross-language Quality reports

AI Engineering OS uses structured failure evidence whenever a supported test runner can produce it deterministically.

The goal is not merely prettier output. Structured failures allow baseline-aware Quality to distinguish:

- pre-existing failures;
- new regressions;
- changed failure causes;
- resolved failures;
- incomplete/unparseable evidence.

## Report isolation

Machine-readable reports are written under the verifier's temporary isolated cache, never inside the staged workspace.

The lifecycle is:

~~~text
create isolated verification cache
  -> materialize report path
  -> run trusted project verification command
  -> parse complete machine-readable report
  -> attach normalized failures
  -> remove isolated cache
~~~

This prevents report files from:

- appearing as staged source changes;
- expanding an agent's allowed file scope;
- affecting promotion diffs;
- remaining in the developer repository.

## Go

~~~text
go test -json ./...
~~~

The native Go event stream remains the preferred source.

## Java

Maven/Gradle/JUnit terminal output is normalized into stable test/build failure identities when the parser can explain the failed exit completely.

## Jest and Vitest

Jest and direct Vitest scripts now write their JSON reporter output to an isolated file.

~~~text
Jest:
  --json --outputFile=<isolated-report>

Vitest:
  --reporter=json --outputFile=<isolated-report>
~~~

Vitest's JSON format is Jest-compatible, so both use the same normalized parser.

## Angular

For `ng test`, AI Engineering OS inspects `angular.json`.

If a test target uses:

~~~text
@angular/build:unit-test
~~~

the verifier requests Angular's built-in JSON reporter and isolated output file:

~~~text
--watch=false --reporters=json --output-file=<isolated-report>
~~~

Current Angular unit-test projects use Vitest by default, and the report is consumed through the same Jest-compatible JSON parser.

Legacy/unknown Karma builders still execute deterministically with `--watch=false`, but the verifier does not falsely claim structured parse completeness.

## pytest

pytest has built-in JUnit XML output and does not require an extra plugin:

~~~text
python -m pytest -r fE --junitxml=<isolated-report>
~~~

Failures and setup/runtime errors are normalized into stable IDs using the test file and test name when available.

The parser compares the JUnit failure/error counts with the extracted failure records. Missing evidence yields `partial` or `failed`, never a false `complete`.

## Safety rule

A failed check can participate in intelligent baseline comparison only when its structured evidence is complete and unambiguous.

If a report is:

- absent;
- malformed;
- incomplete;
- inconsistent with failure counts;

Quality fails closed and does not silently treat unknown failures as pre-existing.

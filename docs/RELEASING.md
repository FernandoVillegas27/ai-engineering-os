# Releasing AI Engineering OS

## Version line

Current release candidate:

~~~text
0.1.0-rc.1
~~~

The source default lives in `core/buildinfo`.

Release builds should inject immutable build metadata through Go linker variables rather than editing source for every artifact.

## Required pre-release evidence

Do not tag a release candidate until:

1. the pull request matrix is green on Linux, Windows and macOS;
2. `go test -race ./...` is green on Linux;
3. `go build ./...` and `go vet ./...` are green;
4. doctor/version smoke tests are green;
5. release-readiness blockers for the intended autonomy scope are satisfied;
6. the release candidate is built from an exact master commit.

A tag is evidence of a chosen source revision, not permission to weaken runtime safety boundaries.

## Reproducible build metadata

Unix-like example:

~~~sh
VERSION=0.1.0-rc.1
COMMIT=$(git rev-parse HEAD)
DATE=$(git show -s --format=%cI HEAD)

go build \
  -trimpath \
  -ldflags "-X github.com/FernandoVillegas27/ai-engineering-os/core/buildinfo.Version=$VERSION -X github.com/FernandoVillegas27/ai-engineering-os/core/buildinfo.Commit=$COMMIT -X github.com/FernandoVillegas27/ai-engineering-os/core/buildinfo.Date=$DATE" \
  -o ai ./cmd/ai
~~~

Verify:

~~~sh
./ai version --json
~~~

The reported commit must match the intended release source.

## Artifact targets

The core is currently verified by CI on:

- Linux amd64 runner;
- Windows amd64 runner;
- macOS GitHub-hosted runner.

A tagged release publishes independently verifiable installable artifacts for:

- CLI linux/amd64;
- CLI windows/amd64;
- CLI darwin/amd64;
- CLI darwin/arm64;
- VS Code client as a `.vsix`;
- IntelliJ IDEA client as a plugin distribution `.zip`.

Every package is accompanied by its own `.sha256` file. The release job verifies every package checksum again and generates one combined `SHA256SUMS` manifest before publication.

Build helpers:

~~~sh
VERSION=0.1.0-rc.1 bash scripts/build-release.sh
VERSION=0.1.0-rc.1 bash scripts/build-vscode-release.sh
VERSION=0.1.0-rc.1 bash scripts/build-intellij-release.sh
~~~

VS Code does not encode SemVer prerelease suffixes in the extension manifest. For an RC product version such as `0.1.0-rc.1`, the VSIX keeps manifest version `0.1.0` and is packaged with VS Code's prerelease marker; the filename retains the complete product version.

A public binary release should publish separate platform/client artifacts rather than pretending one package is portable across operating systems or IDEs.

## Release checklist

- update `README.md` and compatibility docs if support changed;
- update `docs/RELEASE_READINESS.md`;
- ensure persisted schema changes are documented in `MIGRATION.md`;
- ensure no provider/model was pinned per agent role;
- ensure optional provider absence still degrades visibly rather than failing open;
- verify original-tree promotion remains approval-gated;
- run the full CI matrix from the exact candidate commit;
- record checksums for all CLI and IDE packages;
- verify the VSIX and IntelliJ distribution were built from the exact candidate source;
- create the Git tag only after evidence is green.

## Tag convention

Release candidates:

~~~text
v0.1.0-rc.1
v0.1.0-rc.2
~~~

Stable release:

~~~text
v0.1.0
~~~

Do not move an existing release tag. A corrected build gets a new version/tag.

## Post-release

For every published release retain:

- source commit;
- tag;
- platform;
- build metadata;
- artifact checksum;
- compatibility matrix;
- migration notes;
- known limitations.

This makes a release independently inspectable instead of relying on maintainer-local state.

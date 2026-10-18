# Contributing to minigit

Thank you for your interest in contributing to `minigit`! We welcome contributions, bug fixes, architecture improvements, and documentation enhancements.

Please read this document before submitting issues or pull requests to understand our development workflows and quality requirements.

---

## Core Principles

1. **Pure Go / Zero External Dependencies**: `minigit` has no third-party Go dependencies. Every algorithm, binary parser, wire protocol, and object serializer is implemented using only the Go standard library. `go.mod` must remain completely dependency-free.
2. **100% Bidirectional Git Interoperability**: Any repository created or modified by `minigit` must pass `git fsck --full` with zero errors, and any repository created by standard `git` must be inspectable and modifiable by `minigit`.
3. **Mechanical Sympathy**: Git is a content-addressable object filesystem. Byte-level layout, big-endian integer encodings, index stage bitmasks, and SHA-1 checksumming must strictly match official Git specifications.

---

## Development Prerequisites

- **Go**: Version `1.22` or later.
- **Git**: Official Git CLI (`git` version 2.30+), used by the integration test suite to verify cross-compatibility and run `git fsck`.
- **Make / Bash**: For local automation (macOS, Linux, or WSL).

---

## Getting Started

### 1. Clone & Build

```bash
git clone https://github.com/krabhi4/minigit.git
cd minigit

# Compile binary locally
go build -o minigit .

# Verify installation
./minigit --help
```

### 2. Running Test Suites

Run the full test suite with race detector enabled:

```bash
go test -v -race ./...
```

Run specific test packages:

```bash
# Myers diff & line merge tests
go test -v ./pkg/diff

# .gitignore pattern parser and matcher tests
go test -v ./pkg/ignore

# Git index v2 binary reader/writer tests
go test -v ./pkg/index

# Object store (blob/tree/commit) tests
go test -v ./pkg/object

# Packfile v2 & OFS/REF delta decoder tests
go test -v ./pkg/pack

# Repository operations (init, clone, merge, status, branches)
go test -v ./pkg/repo

# End-to-end integration & git fsck verification tests
go test -v ./test
```

---

## Development Workflow

### Branching Strategy
- Create dedicated topic branches from `main`:
  ```bash
  git checkout -b feat/pack-thin-deltas
  # or
  git checkout -b fix/detached-head-status
  ```

### Code Formatting & Quality
Before committing, ensure your code conforms to standard Go guidelines:

1. **Format Code**:
   ```bash
   gofmt -s -w .
   ```
2. **Run Linter / Vet**:
   ```bash
   go vet ./...
   ```
3. **Verify Zero Regressions**:
   ```bash
   go test -count=1 -race ./...
   ```

---

## Testing Requirements

Every pull request must include tests demonstrating correctness:

- **Unit Tests**: Place unit tests alongside the code in `pkg/<module>/<module>_test.go`.
- **Integration Tests**: Place multi-step CLI workflows and cross-compatibility checks in `test/integration_test.go`.
- **Cross-Compatibility Tests**: When touching Git storage formats (index, packfiles, trees, commits), test bidirectional compatibility against the official `git` binary (e.g. running `git fsck --full`, `git gc`, or comparing `git diff` / `git status` output).

---

## Pull Request Guidelines

1. Ensure the CI workflow passes on GitHub Actions.
2. Keep commit history clean and meaningful.
3. Reference any relevant issue numbers in the PR description (e.g., `Fixes #12`).
4. Avoid submitting large binary files or build artifacts (`minigit`, `.DS_Store`, etc.).

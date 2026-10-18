# minigit

[![CI](https://github.com/krabhi4/minigit/actions/workflows/ci.yml/badge.svg)](https://github.com/krabhi4/minigit/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.22-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Zero Dependencies](https://img.shields.io/badge/dependencies-0-brightgreen.svg)](go.mod)
[![Git fsck](https://img.shields.io/badge/git%20fsck---full-passing-success.svg)](test/integration_test.go)

A complete, self-contained Git implementation built in pure Go with **zero external dependencies**.

`minigit` implements core Git internals, binary wire/disk formats, and content-addressable storage algorithms from the ground up, achieving bidirectional compatibility with the official `git` binary. Repositories created with `minigit` pass `git fsck --full` with zero errors.

---

## Table of Contents

- [Features](#features)
- [Architecture & Internals](#architecture--internals)
- [Installation & Quick Start](#installation--quick-start)
- [Command Reference](#command-reference)
- [Usage Walkthrough](#usage-walkthrough)
- [Cross-Compatibility Verification](#cross-compatibility-verification)
- [Testing](#testing)
- [Contributing](#contributing)
- [License](#license)

---

## Features

- **Content-Addressable Object Database**: Loose object storage with zlib compression (`DEFLATE`) and SHA-1 hashing for `blob`, `tree`, `commit`, and `tag` objects.
- **Packfile Engine**: Full decoder for Git packfile format v2 (`.pack`) and index v2 (`.idx`), including OFS-delta and REF-delta reconstruction, allowing seamless inspection after `git gc`.
- **Packed References Engine**: Transparent parsing and priority resolution for `.git/packed-refs` with annotated tag peel markers (`^<hash>`).
- **Binary Index Management**: Direct binary parser and serializer for Git Index Format v2 (`.git/index`), supporting stat caching, file mode bits, SHA-1 checksum verification, and multi-stage resolution (stages 0, 1, 2, 3).
- **Hierarchical Trees & Symlinks**: Tree builder honoring strict Git canonical sorting (where directory tree entries sort as if followed by `/`), directory nesting, and symlinks (`mode 120000`).
- **Myers Diff Algorithm**: Native implementation of the Myers $O(ND)$ greedy shortest edit path algorithm, producing standard unified diff hunks (`--- a/...`, `+++ b/...`, `@@ -x,y +x,y @@`).
- **Chunk-Level 3-Way Merge**: Merge-base calculation via lowest common ancestor graph traversal, chunk-level line merging, automatic clean merges for non-overlapping edits, conflict markers (`<<<<<<< HEAD`, `=======`, `>>>>>>>`), and merge abort capability (`minigit merge --abort`).
- **Binary & Rename Detection**: Fast null-byte heuristic for binary file detection, similarity index scoring, and rename diff formatting (`similarity index 100%`, `rename from ...`).
- **Comprehensive `.gitignore` Support**: Nested `.gitignore` processing across subdirectories, anchored patterns, directory-only matches (`dir/`), wildcards (`*`, `?`, `**`), character ranges (`[...]`), negation rules (`!pattern`), and forced staging (`-f`).
- **Subdirectory Execution**: Execute commands seamlessly from any nested subdirectory within a repository with automatic path canonicalization.
- **Clone Engine**: Clone local repositories with full object store copying, reference resolution, HEAD state reconstruction, and working tree checkout.
- **100% Zero Third-Party Dependencies**: Pure Go using only the standard library (`io`, `os`, `path/filepath`, `compress/zlib`, `crypto/sha1`, `encoding/binary`).

---

## Architecture & Internals

`minigit` is structured into focused, modular packages reflecting Git's internal architecture:

```
minigit/
├── main.go                     # CLI entry point, argument parsing, output formatting
├── go.mod                      # Module definition (zero third-party dependencies)
├── pkg/
│   ├── object/                 # Git loose object model (blob, tree, commit, tag)
│   ├── pack/                   # Packfile v2 reader, index v2 reader, delta resolution
│   ├── index/                  # Binary index v2 reader & writer (.git/index)
│   ├── diff/                   # Myers diff algorithm, similarity index, 3-way merge
│   ├── ignore/                 # Nested .gitignore parser, glob engine, pattern matcher
│   └── repo/                   # High-level repo operations, DAG graph traversal, workdir sync
└── test/
    └── integration_test.go     # End-to-end test suite and official git cross-checks
```

### Component Roles

| Package | Purpose | Standard File Format |
|---|---|---|
| `pkg/object` | Object serialization, parsing, and loose storage | `.git/objects/??/*` |
| `pkg/pack` | Reading compressed packfiles and index v2 delta trees | `.git/objects/pack/*.pack`, `*.idx` |
| `pkg/index` | Reading and writing binary stage metadata | `.git/index` |
| `pkg/diff` | Computing minimal line diffs and performing 3-way line merges | Unified diff patch format |
| `pkg/ignore` | Evaluating ignore rules hierarchically | Root and nested `.gitignore` files |
| `pkg/repo` | Coordinating HEAD, branches, tags, merge operations, and worktree | `.git/HEAD`, `.git/refs/*`, `.git/packed-refs` |

---

## Installation & Quick Start

### Prerequisites
- Go `1.22` or later
- Official `git` binary (optional, for running cross-compatibility verification tests)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/krabhi4/minigit.git
cd minigit

# Compile the binary
go build -o minigit .

# Verify installation
./minigit --help
```

---

## Command Reference

| Command | Synopsis | Description |
|---|---|---|
| `init` | `minigit init [dir]` | Initialize an empty Git repository |
| `clone` | `minigit clone <src> [<dst>]` | Clone a repository into a new directory |
| `status` | `minigit status` | Show staged, unstaged, unmerged, and untracked changes |
| `add` | `minigit add [-f\|--force] <file>...` | Stage file modifications to the index |
| `rm` | `minigit rm [--cached] <file>...` | Remove files from the working tree and/or index |
| `commit` | `minigit commit -m <msg> [--author <name <email>>]` | Record staged snapshot as a new commit |
| `log` | `minigit log [--oneline] [-n <N>] [<rev>]` | Display commit history DAG |
| `diff` | `minigit diff [--staged\|--cached] [<commit>] [<commit2>]` | Show unified diff of workdir, staged changes, or between commits |
| `branch` | `minigit branch [-d <name>] [<name>] [<start-point>]` | List, create, or delete branches |
| `checkout` | `minigit checkout <branch-or-commit>` | Switch branches or checkout specific revisions |
| `merge` | `minigit merge [--abort] <branch-or-commit>` | Merge target branch into current branch |
| `tag` | `minigit tag [-d <name>] [<name>] [<target>]` | List, create, or delete lightweight/annotated tags |
| `hash-object` | `minigit hash-object [-w] [-t <type>] <file>` | Compute object ID and optionally store object |
| `cat-file` | `minigit cat-file (-p\|-t\|-s) <object>` | Inspect object contents, type, or byte size |
| `write-tree` | `minigit write-tree` | Build a tree object from current index stages |
| `version` | `minigit version` | Display version information |

---

## Usage Walkthrough

### 1. Initialize and Commit

```bash
minigit init my-repo
cd my-repo

echo "Hello world" > greeting.txt
minigit status
# Shows 'greeting.txt' as untracked

minigit add greeting.txt
minigit commit -m "Initial commit"
# [main e1a4b52] Initial commit
```

### 2. Branching & Merging

```bash
# Create and switch to a feature branch
minigit branch feature-updates
minigit checkout feature-updates

echo "New exciting feature" >> greeting.txt
minigit add greeting.txt
minigit commit -m "Add feature updates"

# Switch back and merge
minigit checkout main
minigit merge feature-updates
# Fast-forward merge or 3-way merge completed
```

### 3. Handling Conflicts & Merge Abort

When concurrent modifications overlap:

```bash
minigit merge conflicting-branch
# Automatic merge failed; fix conflicts and then commit the result.

minigit status
# You have unmerged paths:
#     both modified:   greeting.txt

# Inspect conflict markers in file, resolve and stage:
minigit add greeting.txt
minigit commit -m "Resolve merge conflict"

# Or abort the merge and restore previous state:
# minigit merge --abort
```

### 4. Working with `.gitignore`

```bash
# Add patterns to .gitignore
echo "*.log" > .gitignore
echo "secret/" >> .gitignore

# Staging respects rules automatically
touch debug.log
minigit status
# debug.log is omitted from untracked files

# Force-add an ignored file if explicitly needed
minigit add -f debug.log
```

---

## Cross-Compatibility Verification

`minigit` repositories are 100% wire- and disk-compatible with standard Git. You can verify this at any point using the official `git` command line tools:

```bash
# Initialize with minigit and create commits
minigit init test-repo
cd test-repo
echo "Testing compatibility" > file.txt
minigit add file.txt
minigit commit -m "Created with minigit"

# Run official git fsck to verify object database integrity
git fsck --full
# Output: notice: empty / clean check, 0 dangling errors

# Inspect history using official git
git log --oneline
# Shows commit history identically

# Run git garbage collection to create packfiles
git gc
# minigit seamlessly reads the generated .pack and .idx files!
minigit log --oneline
```

---

## Testing

The project includes an extensive automated test suite covering unit behavior, binary parsing, Myers diff edge cases, merge conflict resolution, and end-to-end integration with the official `git` binary:

```bash
# Run all tests with Go race detection
go test -v -race ./...

# Run unit tests only
go test -v ./pkg/...

# Run integration tests only
go test -v ./test
```

---

## Contributing

Contributions are welcome! Please review [CONTRIBUTING.md](CONTRIBUTING.md) for details on our workflow, testing requirements, and zero-external-dependency policy.

All participants are expected to adhere to our [Code of Conduct](CODE_OF_CONDUCT.md).

---

## License

This project is licensed under the terms of the MIT License. See [LICENSE](LICENSE) for details.

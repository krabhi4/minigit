# minigit

A self-contained Git implementation built in pure Go with zero external dependencies.

`minigit` implements core Git internals and wire/disk formats from scratch, achieving bidirectional compatibility with the official `git` binary (passing `git fsck --full`).

## Features

- **Object Database**: Loose object storage with zlib compression and SHA-1 hashing for `blob`, `tree`, and `commit` objects.
- **Packfile Engine**: Parsing of Git packfiles (`.pack`) and index v2 (`.idx`) including OFS/REF delta reconstruction, enabling reading repositories after `git gc`.
- **Packed References**: Transparent resolution and listing of packed refs (`.git/packed-refs`).
- **Index Management**: Binary parser and writer for Git index format v2 (`.git/index`) with stage sorting and SHA-1 checksum verification.
- **Hierarchical Trees**: Tree serializer supporting proper Git sorting order (treating tree entries with trailing slash for sorting), symlinks (`mode 120000`), and nested directories.
- **Myers Diff Algorithm**: Native implementation of the Myers diff algorithm producing standard unified diff output compliant with patch/git apply (`--- a/...`, `+++ b/...`, `@@ -x,y +x,y @@`).
- **3-Way Merge**: Merge base detection via graph traversal, fast-forward merges, 3-way tree merges with two-parent merge commits, and conflict marker emission (`<<<<<<< HEAD`, `=======`, `>>>>>>>`).
- **DAG Traversal**: Commit history navigation with parent pointers, author metadata, and formatted logs.
- **Branch & Checkout**: Reference resolution for symbolic (`refs/heads/*`) and detached `HEAD`, branch switching, working tree synchronization, and index regeneration.
- **Subdirectory Execution**: Full support for running commands from any subdirectory inside a repository.
- **Full Git Interoperability**: Repositories created with `minigit` can be inspected and committed to with real `git`, and vice-versa.

## Commands

```bash
minigit init [dir]
minigit hash-object [-w] <file>
minigit cat-file (-p|-t|-s) <object>
minigit add <file>...
minigit rm [--cached] <file>...
minigit write-tree
minigit commit -m <msg> [--author <name <email>>]
minigit status
minigit log [--oneline] [-n <count>] [<revision>]
minigit branch [-d <name>] [<name>]
minigit checkout <branch-or-commit>
minigit diff [--cached|--staged]
minigit merge <branch-or-commit>
minigit tag [-d <name>] [<name>]
```

## Building & Testing

```bash
go build -o minigit .
go test -v ./...
```

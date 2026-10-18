---
name: Bug Report
about: Create a report to help us improve minigit
title: '[BUG] '
labels: ['bug']
assignees: ''
---

**Describe the bug**
A clear and concise description of what the bug is.

**To Reproduce**
Steps to reproduce the behavior:
1. Initialize repository with `minigit init`
2. Create files / commit `...`
3. Run command `minigit ...`
4. See error

**Expected behavior**
A clear and concise description of what you expected to happen (and how it compares to official `git` output).

**Observed Output**
```
$ minigit ...
<paste error output here>
```

**Git Cross-Compatibility**
- Did you verify this with official `git`? (Yes / No)
- Output of official `git fsck --full`:
```
<paste git fsck output here>
```

**Environment (please complete the following information):**
- OS: [e.g. macOS, Ubuntu 22.04, Windows 11]
- Go Version: [e.g. 1.22.x, 1.23.x]
- Architecture: [e.g. arm64, amd64]

**Additional context**
Add any other context about the problem here (e.g., reproduction repository or minimal test case).

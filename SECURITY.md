# Security Policy

## Supported Versions

Only the latest release and the current `main` branch receive security patches.

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| main    | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take the security of `minigit` seriously. Because `minigit` directly parses and manipulates file formats, binary packfiles, index records, and repository objects, memory safety and canonical path traversal boundaries are critical.

If you believe you have discovered a security vulnerability in `minigit`, please **do NOT report it via public GitHub issues or discussions**.

Instead, please report security vulnerabilities using one of the following methods:

1. **GitHub Private Vulnerability Reporting**: Submit a private advisory directly on GitHub via the **Security** tab -> **Report a vulnerability**.
2. **Security Contact**: Email the maintainers directly with details of the vulnerability.

### When reporting, please provide:
- A clear description of the vulnerability and its potential impact.
- Step-by-step reproduction instructions or a minimal test case / proof-of-concept repository.
- Any suggested mitigations or patches if available.

### Disclosure Process
- You will receive an acknowledgment within 48 hours.
- We will validate the issue, prepare a patch, and coordinate release timing.
- We kindly request that you observe coordinated disclosure until a patch is publicly available.

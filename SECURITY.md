# Security Policy

This is a personal Kubernetes training and portfolio project. The only runnable
code is the Go service under [`flashcards/`](flashcards/), which is meant to run
locally on KIND and is not a hosted or production service.

## Supported versions

There are no tagged releases. Security fixes, if any, land on the `main` branch;
that is the only supported line.

## Reporting a vulnerability

Please report suspected vulnerabilities **privately** — do not open a public
issue for a security problem.

Use GitHub's private vulnerability reporting: open the repository's **Security**
tab and choose **Report a vulnerability**. This keeps the report visible only to
the maintainer until a fix is ready.

Please include enough detail to reproduce the issue (affected file or endpoint,
steps, and impact). Because this is a solo project, expect a best-effort
response rather than a guaranteed timeline.

## What already runs

`flashcards/` gates every change on `make check`, which includes `gosec` static
analysis, `govulncheck` dependency scanning, and `gitleaks` secret scanning. A
report that one of these tools misses something concrete is especially welcome.

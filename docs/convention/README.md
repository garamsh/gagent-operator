# Conventions Index

Rules every agent follows when writing code, tests, commits, and documents. The PM owns these files; workers read and apply them.

## Contents
- Stack-neutral
- Stack-specific
- Independence
- Precedence

## Stack-neutral

| File | Governs |
|---|---|
| `code-comments.md` | When and how to comment code |
| `runtime-safety.md` | Types, trust boundaries, error handling |
| `testing.md` | Test layers, mocking, placement |
| `ci.md` | Local/CI parity, pipeline authoring, security |
| `git.md` | Branches, commits, PR titles, merging |
| `style.md` | Naming and pattern consistency in code |
| `simplicity.md` | Whether to add or to change the shape; when an abstraction earns its place |
| `review.md` | What a review must carry to be valid, and how an author responds |
| `documentation.md` | What makes a document or a GitHub artifact valid, review comments included |

Every project keeps all of these. They are the floor, and bootstrap prunes only the table below.

Selection is not once. A project that changes its stack or its structure takes the file that now matches and drops the one that no longer does, in the pull request that makes the change.

This project has no architecture file. Its shape is fixed by the kubebuilder scaffold — an entry point, an API package, and a controller package — rather than partitioned into boundaries a document would have to describe, so no `arch-*.md` applies.

## Stack-specific

These apply only when the project uses that stack.

| File | Governs |
|---|---|
| `stack-kubebuilder.md` | Go operators scaffolded by kubebuilder: layout, API types, controllers, markers, logging, tests |

A stack file states the concrete form of what a stack-neutral file governs: the test runner and file placement behind `testing.md`, the comment syntax behind `code-comments.md`, the commands behind an entry-point name in `ci.md`. It never restates the rule itself. This table is where that split is recorded — the files do not point at each other.

`stack-kubebuilder.md` owns every Go rule here, the general ones included. This project has no separate `stack-go.md`: a second file governing the same `.go` files would break the one-file-one-territory rule below, and the operator's layout, logging, and test rules are not the general Go ones.

## Independence

Each convention file is self-contained: reading it alone is enough to apply its rules.

- **One file, one territory.** No artifact is governed by two files. Where two files could both decide a case, one of them is holding the wrong rule.
- **A rule appears once** — inside a file as much as across files. A section that restates earlier rules in the negative is a second site to keep in sync, not a summary.
- **Two files' rules may share a reason.** That is not duplication. Each states its own reason; neither points at the other for it.
- **A convention file does not send the reader to another convention file.** Where one file's territory ends and the next begins is recorded in the table above, not inside the files themselves.

## Precedence

1. The stack-specific file, when one applies.
2. The stack-neutral files.
3. This index.

On conflict, the more specific rule wins. Report conflicts to the PM in the PR description — do not resolve them yourself.

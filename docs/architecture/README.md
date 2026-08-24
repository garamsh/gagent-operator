# Architecture

Current architecture of the system and the decisions behind it. This file is the index; workers read it before touching code, the PM keeps it honest.

## Structure

- **Responsibility documents** (one `.md` per domain or concern, e.g. `memory.md`, `gateway.md`) — the single source of truth for how the system is shaped *now*. To know the current state, read only these.
- **`adr/`** — the record of individual decisions: direction taken, context, rejected alternatives.

## Rules

1. **Responsibility document skeleton**: current decisions (consolidated) / rationale summary with links to the relevant ADRs / open questions. Do not copy ADR content — synthesize the present state.
2. **ADRs are append-only.** Once merged, the body is frozen. Exceptions: updating the status field (`accepted` → `superseded by ADR-XXXX`), fixing typos or broken links, and appending a dated entry under `## Errata` when the decision stands but a fact supporting it was wrong — the erratum names what falsified it, and the original text stays intact. A changed decision means a new ADR that supersedes the old one — never an edit.
3. **Decisions land in pairs.** A PR that adds or supersedes an ADR must update the affected responsibility documents in the same PR. A PR with only one of the two is rejected — the final state must always live in the responsibility documents.
4. Keep this index current: every responsibility document and every ADR is listed here. An ADR covers a decision that changes a project rule — a settled choice that does not affect the rules does not earn one.
5. **Every domain or concern has a responsibility document.** The PR that introduces one adds its document; the PR that removes one deletes it. A system with no code yet has none, and that is the correct state.

## Index

### Responsibility documents

| Document | Covers |
|---|---|
| `agent.md` | The `agent.garam.sh` API group, the `Agent` kind, and its controller |

### ADRs

| ADR | Decision | Status |
|---|---|---|
| `adr/0001-kubebuilder-go-v4-scaffold.md` | Build the operator on the kubebuilder go/v4 scaffold | accepted |
| `adr/0002-dev-integration-branch.md` | Integrate on `dev` and keep `main` for released state | accepted |
| `adr/0003-single-stack-convention-file.md` | Govern Go with one stack file written for the operator | superseded by ADR-0004 |
| `adr/0004-extend-stack-go.md` | Extend `stack-go.md` instead of replacing it | accepted |
| `adr/0005-statefulset-of-one.md` | Run an agent as a StatefulSet of one replica | accepted |
| `adr/0006-credential-group.md` | Carry credential access on a group, not on a user | accepted |
| `adr/0007-claim-definitions-from-a-poller.md` | Claim garam's definitions from a poller beside the reconciler | accepted |

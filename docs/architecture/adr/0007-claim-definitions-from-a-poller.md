# ADR 0007: Claim garam's definitions from a poller beside the reconciler

> Status: accepted
> Date: 2026-08-24

Append-only: once merged, the body below is not rewritten. A fact later found wrong is corrected in an appended note, not edited out; a revised decision is a new ADR that supersedes this one.

## Context

garam decides how an operator learns which agents it is to run, and `garam@8f9dd9d:docs/architecture/adr/0033-an-operator-claims-the-agent-a-definition-mints.md` (`accepted`) is where: the machine listener answers an operator with the definitions naming it, polled rather than watched; claiming binds the operator to the agent GRN the definition minted and is what admits it to that agent's certificate route; a definition is claimed once, and a second claim of the same GRN is a conflict rather than an override. The surface is built — `garam@8f9dd9d:internal/machineapi/definition.go` serves `GET /definitions` and `POST /definitions/{agent}/claim`, and the read is scoped by the definition's own operator field, so it can name nothing the caller is not named in. It answers at `garam-machine.garam.svc.cluster.local:8443`, a ClusterIP Service, over mutual TLS; nothing stands between an operator in the same cluster and it.

What that leaves this repository is where the poll runs, what this operator authenticates as, what it verifies garam against, and what it does with a claim once it holds one.

**What identifies this operator and what verifies garam are different files.** An operator's own certificate is minted out of band and arrives as three: `certificate.pem`, `key.pem` and `issuer.pem` (`garam@8f9dd9d:internal/cli/issue_certificate.go:98-100`). The third is the *organization's* authority — the one that signed this operator — and not the one that signs garam's listener. `AgentSpec.credentialsSecretName` is a third thing again: an agent's credential, referenced per `Agent` and mounted into its Pod.

**garam's listener presents a certificate no organization issuer verifies.** Measured on garam-dev on 2026-08-24: it serves a self-signed certificate, `CN=garam-machine.garam.svc.cluster.local`, `CA:FALSE`, carrying the Service's DNS names. That is not the per-organization certificate garam's ADR-0037 decides, and closing the gap is their #482 and #483, neither landed. A self-signed certificate is still a certificate: Go verifies a leaf that is itself in the root pool as a one-element chain (`crypto/x509/verify.go:593`, go1.26.0). So what an operator trusts is one file whose *contents* change when garam's server root lands — not a mechanism that has to be replaced with it, and not a reason to skip verification in the meantime.

**Expiry is the whole of garam's revocation.** It publishes no revocation list and runs no responder, so lifetimes carry that job and will be short.

**What a definition carries is not what an `Agent` requires.** `garam@8f9dd9d:api/openapi.yaml:1606` bounds `AgentValues`: `map[string]string`, a key of 1 to 64 characters of lowercase letters, digits and `-`, a value of at most 4096 bytes, at most 32 entries. garam stores the keys and the strings and interprets neither, so what they mean is this operator's contract rather than garam's. The keys expected are `model`, `system-prompt`, `workspace`, `model-credential`, `repo-credential` — `system-prompt` and never `systemPrompt`, which the key rule refuses — and a credential key carries the *name* of a credential and never its value. `AgentSpec` requires an image reference and a storage size, and no value carries either.

## Decision

**The poll runs beside the reconcilers, as a `manager.Runnable`.** What wakes a poll is the clock and not an event on an `Agent`, and a reconciler that polled would need an object to hang a requeue on — which a definition this operator has not claimed does not have here. A `Runnable` added to the manager joins its leader-election group (`sigs.k8s.io/controller-runtime@v0.24.1:pkg/manager/runnable_group.go:99`), so the deployment's `--leader-elect` is what keeps two processes from racing for one claim.

**What this operator authenticates as and what it verifies garam against are three file paths, given as process configuration.** `--garam-certificate-file` and `--garam-key-file` are its identity; `--garam-trust-file` is what garam's listener is verified against, and is not the organization issuer that arrives beside them. None is compiled in and none has a default: what an operator trusts is the deployment's to supply, and both outcomes of garam's #482 change that file's contents rather than this shape. `InsecureSkipVerify` is set nowhere and no flag offers it, because verification succeeds today against the certificate garam presents today.

**The certificate is read at each handshake, through `GetClientCertificate`.** A configuration holding one certificate stops authenticating on a schedule, and a short lifetime is the only revocation there is. Re-reading is not sufficient on its own: a reused connection performs no handshake, so this client keeps none alive between calls. Nor does either make a file change — see the consequences.

**A claimed agent GRN is reported in `AgentStatus`, never written in `AgentSpec`.** A GRN garam minted is not something a user writes, which is what Spec holds; and it needs no durability here, because garam is where the claim is durable — a read answers the claim state, so an operator that lost its status entirely still learns what it holds. The field is not added by this change: nothing writes it yet, and a field written by nothing is the addition `simplicity.md` refuses. Whichever change first fills it adds it, in the same change.

**A claim refused as a conflict is terminal.** Nothing retries it: the pass moves on to the next definition, and the next read reports the definition as claimed. Once an `Agent` carries the claim, that refusal is a `Claimed` condition of `False` with reason `ClaimConflict` — a reason this API does not have, added by the change that sets it.

**Reading nothing is the default.** With `--garam-address` unset the operator makes no outbound call at all, which is the state a deployment is in until it has been given an identity. That is also what keeps this change landable while garam's #483 is open: the shape is settled and only values wait.

## Consequences

Easier: the whole path is exercisable now. A trust file and an operator certificate are all that separate a deployment from polling, and the shape survives #483 — an ordinary server certificate for a real hostname and a pinned garam root are the same three flags with different bytes behind them.

Harder: three files have to reach the manager's Pod before it does anything, and this repository names none of the mechanics. The certificate a `GetClientCertificate` callback re-reads is only as fresh as whatever rewrites it, and **this operator has no renewal mechanism and no obligation on the deployment stated anywhere**. A callback re-reading a file nothing ever rewrites reads the same bytes until they expire, and looks exactly like working rotation until then. That is an open question and it is recorded as one in `agent.md` rather than answered with a mechanism invented here.

Ruled out: `InsecureSkipVerify`, in every form including an off-by-default flag — nothing needs it, and a switch that disables verification is what nobody removes later. Ruled out with it: compiling a trust bundle into the operator, and reading the trust material out of the organization issuer an operator's certificate arrives with, which is a different authority.

Not decided here: **whether a definition supplies everything an `Agent` needs or only its composition.** `AgentValues` carries no image and no storage size, so nothing in this change constructs an `Agent` from a claimed definition — the claim is made and reported, and no object is created or bound. The two branches cost different things and the choice is garam's contract as much as this repository's; `agent.md` §Open questions holds both.

Not decided here either: the poll interval as anything but a default, metrics for the poll, and what the operator reports back to garam. Nothing needs them yet.

# Configuration

How this operator's deployment is configured, and which repository owns each value.

## Current decisions

- The operator is configured by flags on the manager's command line and by nothing else. `cmd/main.go` parses them; no file, ConfigMap, or environment variable carries a value a deployer chooses. `POD_NAMESPACE` is the single environment variable and is not such a value — the downward API supplies the Pod's own namespace, because the kustomize transformer that sets it does not reach into an argument.
- Every flag has exactly one owner, and there are two owners: this repository's base under `config/`, and the overlay that deploys that base. A flag neither sets runs at the default `cmd/main.go` declares, and leaving it unset is the base's decision.
- **The base owns a flag whose value is the same wherever this operator runs.** `config/manager/manager.yaml` sets it, and a deployment that wants another replaces the argument.

| Flag | Why the base owns it |
|---|---|
| `--agent-copy-image` | The init container is in every agent's Pod, so every deployment needs the flag. What it asks of the image is a shell and `install` and nothing of the agent, which no environment answers differently. |
| `--garam-credential-secret` | The base's manager Pod already mounts that Secret by name. The two names have to agree, and an owner that held one of them would leave the pair spanning two repositories. |
| `--garam-certificate-file`, `--garam-key-file` | Paths inside the container, under the `mountPath` the same file writes. Each file's base name is also the Secret key the renewal is written to, so the pair fixes this operator's contract with whoever mints the Secret. |
| `--garam-trust-file` | The same, and it names the role rather than the certificate: what `garam`'s listener is verified against has already moved once, and only the file's contents moved with it. |

- **The deploying overlay owns a flag that names something one cluster holds.** The base sets none of these, and a base that set one would ship one environment's values to every other.

| Flag | Why the overlay owns it |
|---|---|
| `--garam-address` | Names a service in one cluster. It is also the switch: unset leaves this operator reading no definitions and making no outbound call, which is the correct state for a base with no environment behind it. |
| `--agent-image` | Names a registry, a repository and a build. No registry or tag convention exists for the agent image and neither this project nor `garam` owns one, so the base cannot invent a default any more than `spec.image` can. |
| `--agent-storage-size` | How much state an agent keeps depends on the cluster's storage and its agents. It has no default for the same reason: an operator given `garam`'s address and neither this nor an image refuses to start, and a base default would answer that refusal with a guess. |

- **The overlay for a cluster belongs in the repository that reconciles that cluster, and this repository holds no overlay for any environment.** For `admin@garam-dev` that repository is `garamsh/gitops`, whose ADR 0003 already decides the shape: an `Application` under `apps/`, per-cluster values under `clusters/<cluster>/`, and a non-Helm resource included through Kustomize (`gitops@8971dd5:docs/architecture/adr/0003-cluster-aware-app-of-apps-with-per-cluster-overlays.md`). No path under that repository named `gagent` or `gagent-bringup` existed at `gitops@8971dd5`, so that overlay is owed rather than described here.
- **What `config/` exposes so such an overlay can set the three without restating the rest**, and what a change here may therefore not break:
  - `config/default` builds standalone, so a remote overlay or an Argo `Application` can reference it by path and revision.
  - The manager's Deployment carries one container, at index 0 of the Pod spec. `config/default/manager_metrics_patch.yaml` already addresses it that way.
  - The base sets none of the three, so an appended argument is the only occurrence of it and no ordering decides which wins.
  - The manager's image is `controller:latest`, a name a consumer replaces rather than a registry this project claims. `make deploy` replaces it the same way, through a generated overlay under `dist/`.
- **No secret value is configured anywhere.** `--garam-credential-secret` names a Secret, the three credential flags name paths the kubelet writes into, and the material itself is minted out of band and reaches the process as files. Nothing an overlay sets carries key material, and an overlay is not where one would be put.

## Rationale

Where each value lives is [ADR 0013](adr/0013-the-base-carries-what-every-deployment-shares.md), settled on issue #105 after the deployed configuration was measured against this repository: seven of the operator's eight project flags existed in no repository, and the Deployment's field managers were three imperative `kubectl` verbs on top of one apply. It decides the two owners, the test that assigns a flag to one of them, and that an environment's overlay is not this repository's to hold.

That the credential has one home — a Secret mounted into the manager's Pod, read as files and replaced through the API server — is [ADR 0008](adr/0008-renew-the-operator-credential-into-the-secret-it-is-read-from.md). It is why four of the five flags the base now sets are the base's: they are the reading end of a mount `config/manager/manager.yaml` already writes.

That the image and the storage size come from the operator's own configuration rather than from a definition is [ADR 0009](adr/0009-construct-a-claimed-agent-from-the-operators-own-configuration.md). It settles that they are configuration at all; this document settles which repository types them.

## Open questions

- **Nothing reconciles this deployment.** Measured read-only on `admin@garam-dev` on 2026-08-31: the four field managers that had written the Deployment's spec were `kubectl-client-side-apply`, `kubectl-set`, `kubectl-patch` and `kubectl-rollout`, with `kube-controller-manager` beside them writing what it owns and no GitOps controller among any of them. Recording where configuration lives does not apply it, and issue #105 excluded a deploy pipeline deliberately. Until something reconciles it, the live object and this repository can still disagree without anything saying so.
- **The overlay this document names has not been written.** It belongs in `garamsh/gitops` and cannot be created from here. Until it is, `--garam-address`, `--agent-image` and `--agent-storage-size` have an owner and no artifact, which is a smaller gap than having neither but is not the goal issue #105 states.
- **Whether the three intervals are ever an environment's.** `--garam-poll-interval`, `--garam-renewal-interval` and `--garam-report-interval` have defaults and nothing sets them, so the base owns them by omission. The first two are read against windows `garam` measures, so a cluster whose `garam` chose a different window would be what moves them; none has. The third is read against no window at all — `garam` expires no report — so what would move it is a deployment that finds one request per agent per minute too many or a minute of staleness too much.

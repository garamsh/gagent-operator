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
| `--agent-image` | Names a registry, a repository and a build. `gagent` publishes that image and owns its repository and tag scheme (`gagent@b61451e:docs/architecture/adr/0020-immutable-image-repositories.md`), and which build a cluster runs is that cluster's, so the base cannot name one any more than `spec.image` can. |
| `--agent-tools-image` | Names a registry, a repository and a build, as `--agent-image` does. Which tools a cluster's agents are given is that cluster's, and a base naming one would ship one deployment's tool set to every other. Unset is a Pod with no tool tree, which is the correct state for a base with no environment behind it. |
| `--agent-storage-size` | How much state an agent keeps depends on the cluster's storage and its agents. It has no default for the same reason: an operator given `garam`'s address and neither this nor an image refuses to start, and a base default would answer that refusal with a guess. |

- **The overlay for a cluster belongs in the repository that reconciles that cluster; where nothing reconciles it, the overlay lives here under `config/overlays/<environment>/`, and it is removed when something takes it.** A repository that reconciles a cluster does not thereby reconcile this operator in it. `garamsh/gitops` reconciles `admin@garam-dev` and held no path matching `gagent` at `72fff6e0`, none of the ten Argo CD `Application` objects in that cluster names this operator, and the manager's Deployment carries `app.kubernetes.io/managed-by: kustomize` with a `kubectl.kubernetes.io/last-applied-configuration` — it is applied from this repository's `config/` by `kubectl apply`. All read 2026-09-03.
- **`config/overlays/garam-dev/` is the overlay for `admin@garam-dev`, and is the only overlay this repository holds.** It sets three of the four flags above through an appended argument — `--agent-tools-image` is the fourth and names an image no registry holds yet — and it names the operator image that cluster already runs, so that applying it changes those three arguments and nothing else.
- **An image this project does not build is referenced the same way as one it does — `<repository>:<tag>@sha256:<digest>` — and where the registry holds no index, the platform manifest's digest is taken and the architecture it pins is recorded rather than left implicit.** `--agent-image` names `garam/gagent-agent:0.1.0`, an `application/vnd.oci.image.manifest.v1+json` whose config declares `linux/amd64` with no index above it, so this reference pins that architecture; every node of `admin@garam-dev` was `linux/amd64` on 2026-09-03, and the pin is one environment's overlay's rather than the base's. `--agent-copy-image` is the base's and does have an index, so it carries the index digest instead.
- **What `config/` exposes so such an overlay can set those without restating the rest**, and what a change here may therefore not break:
  - `config/default` builds standalone, so a remote overlay or an Argo `Application` can reference it by path and revision.
  - The manager's Deployment carries one container, at index 0 of the Pod spec. `config/default/manager_metrics_patch.yaml` already addresses it that way.
  - The base sets none of them, so an appended argument is the only occurrence of a flag and no ordering decides which wins.
  - The manager's image is `controller:latest`, a name a consumer replaces rather than a registry this project claims. `make deploy` replaces it the same way, through a generated overlay under `dist/`.
- **No secret value is configured anywhere.** `--garam-credential-secret` names a Secret, the three credential flags name paths the kubelet writes into, and the material itself is minted out of band and reaches the process as files. Nothing an overlay sets carries key material, and an overlay is not where one would be put.

## Rationale

Which of the two owners a flag has is [ADR 0013](adr/0013-the-base-carries-what-every-deployment-shares.md)'s test, settled on issue #105 after the deployed configuration was measured against this repository: seven of the operator's eight project flags existed in no repository, and the Deployment's field managers were three imperative `kubectl` verbs on top of one apply.

Where the overlay itself lives is [ADR 0017](adr/0017-an-unreconciled-environments-values-live-in-an-overlay-here.md), which supersedes ADR 0013 and carries that test forward unchanged. ADR 0013 sent an environment's overlay to the repository reconciling that environment and refused one here; issue #119 found that the repository it named does not reconcile this operator, which leaves an environment nothing reconciles with no home for its values at all. ADR 0017 gives that environment one here and takes it away again when a reconciler appears. It also settles the reference form for an image this project does not build, which [ADR 0014](adr/0014-the-image-repository-is-immutable-and-a-deployment-references-a-digest.md) and `delivery.md` decide only for the image it does.

That the credential has one home — a Secret mounted into the manager's Pod, read as files and replaced through the API server — is [ADR 0008](adr/0008-renew-the-operator-credential-into-the-secret-it-is-read-from.md). It is why four of the five flags the base now sets are the base's: they are the reading end of a mount `config/manager/manager.yaml` already writes.

That the image and the storage size come from the operator's own configuration rather than from a definition is [ADR 0009](adr/0009-construct-a-claimed-agent-from-the-operators-own-configuration.md). It settles that they are configuration at all; this document settles which repository types them.

That the tool tree is one of those values is [ADR 0019](adr/0019-mount-an-agents-tool-tree-from-an-image-this-operator-names.md), which names it in an image the deployment supplies for the same reason the agent's image is: it names a build, and no base can name one for every cluster.

## Open questions

- **Nothing reconciles this deployment.** Measured read-only on `admin@garam-dev` on 2026-08-31 and again on 2026-09-03: the four field managers that had written the Deployment's spec were `kubectl-client-side-apply`, `kubectl-set`, `kubectl-patch` and `kubectl-rollout`, with `kube-controller-manager` beside them writing what it owns and no GitOps controller among any of them. Recording where configuration lives does not apply it, and issue #105 excluded a deploy pipeline deliberately. Until something reconciles it, the live object and this repository can still disagree without anything saying so.
- **The overlay names an operator image, and nothing here keeps it current.** It names the build the cluster runs so that applying it moves the three flags and not the image. An operator image published later and not written into the overlay leaves it describing an older deployment than the one running, with nothing in this repository able to notice — the gap `delivery.md` records for the manual publish path, arriving here.
- **No image exists for `--agent-tools-image` to name.** The tool binaries are `gagent`'s build outputs and no registry holds them, which issue #102 is where a push path is asked for. So `config/overlays/garam-dev/` sets the flag nowhere and every agent that cluster runs carries no tool tree — the flag is owned above and unset, which is a deployment waiting on an artifact rather than a value nobody has typed.
- **Whether the three intervals are ever an environment's.** `--garam-poll-interval`, `--garam-renewal-interval` and `--garam-report-interval` have defaults and nothing sets them, so the base owns them by omission. The first two are read against windows `garam` measures, so a cluster whose `garam` chose a different window would be what moves them; none has. The third is read against no window at all — `garam` expires no report — so what would move it is a deployment that finds one request per agent per minute too many or a minute of staleness too much.

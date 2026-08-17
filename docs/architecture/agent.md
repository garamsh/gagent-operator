# Agent

The `agent.garam.sh` API group, the `Agent` kind it serves, and the controller that reconciles it.

## Current decisions

- `Agent` is served at `agent.garam.sh/v1alpha1` and is namespaced. Its types live in `api/v1alpha1/` and its reconciler in `internal/controller/` — the paths the kubebuilder CLI owns.
- `AgentSpec` carries what an agent needs to run: the container image, the Secret its credential material comes from, the persistent volume it keeps state on, and the compute resources its container asks for. `AgentStatus` carries observed state only, and nothing a user writes.
- The image reference is required and has no default. No registry or tag convention exists for the agent image and neither project owns one, so the field asks rather than invents; a default can be added later to a field that already exists.
- Credential material is referenced, never carried. The Secret is created outside this operator, and the workload mounts its keys as files — key material does not travel through environment variables.
- The workload is a StatefulSet of one replica with a volume claim template for the agent's volume.
- An Agent whose credentials Secret is absent gets no workload at all, and the Secret's arrival is what builds it. Mounting a Secret that does not exist leaves the Pod unable to start, and creating that Secret is not an edit to the Agent, so the controller watches the Secret rather than waiting to be woken by a spec change.
- `v1alpha1` makes no outbound call. Nothing here reads a source outside the cluster, and no field names one: an identifier for an external API waits until something fills it.
- The reconciler owns the StatefulSet through an owner reference and holds no finalizer: deleting the Agent is what removes the workload. It writes no status.

## Rationale

The layout and the generated manifests follow the scaffold, per [ADR 0001](adr/0001-kubebuilder-go-v4-scaffold.md): code is written to fit the CLI's paths rather than the CLI's output being reshaped to fit a preferred layout.

The workload's form is [ADR 0005](adr/0005-statefulset-of-one.md): a StatefulSet of one replica, because the agent's state is a single-writer store and the ordering guarantee is the only thing keeping two writers off one volume.

The image field and the absence of outbound calls were settled on issue #11, before the API was written. Neither carries an ADR: the image rule is stated in the field's own doc comment, where it is enforced, and "no outbound call in `v1alpha1`" is a milestone boundary rather than a project rule.

## Open questions

- **Status writing and condition semantics.** Nothing the controller learns reaches the object: an Agent waiting for its Secret, and one whose storage size the workload cannot follow, both read as an Agent with an empty status.
- **The `garam` integration, in both shape and timing.** Which side is the source of truth for the set of agents was answered twice on 2026-08-17, in opposite directions. The first answer — this operator decides and tells `garam` afterwards — described `garam` as it stood that morning and was recorded here. It was superseded the same day: `garam`'s console defines agents, chooses which operator builds each one and with what values, and this operator reads that. The direction is `garam` to operator, and the shape of the surface that carries it — its path, its authentication, whether it is polled or watched, and what key matches an operator to its agents — is undecided at `garam`. A configuration payload will exist there, which the earlier answer also denied.

  What holds across both answers: `garam` never dials inward, an agent's identifier is minted by `garam` and is not known when a CR is created, the per-agent client certificate is issued by `garam` and placed by this operator, and there is no surface for reporting state back.

  Nothing in `v1alpha1` moved when the answer reversed, because no field here names an external system and no code calls one. That was the milestone boundary's purpose and this is the event it was drawn for.
- **Resizing an agent's volume.** A StatefulSet's claim template is immutable after creation, so a change to `spec.storageSize` on an existing `Agent` cannot be satisfied by editing the workload. ADR 0005 accepts the limitation without answering it, and the controller leaves the claim as it is rather than failing.

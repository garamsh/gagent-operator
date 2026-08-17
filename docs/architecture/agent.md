# Agent

The `agent.garam.sh` API group, the `Agent` kind it serves, and the controller that reconciles it.

## Current decisions

- `Agent` is served at `agent.garam.sh/v1alpha1` and is namespaced. Its types live in `api/v1alpha1/` and its reconciler in `internal/controller/` — the paths the kubebuilder CLI owns.
- `AgentSpec` carries what an agent needs to run: the container image, the Secret its credential material comes from, the persistent volume it keeps state on, and the compute resources its container asks for. `AgentStatus` carries observed state only, and nothing a user writes.
- The image reference is required and has no default. No registry or tag convention exists for the agent image and neither project owns one, so the field asks rather than invents; a default can be added later to a field that already exists.
- Credential material is referenced, never carried. The Secret is created outside this operator, and the workload mounts its keys as files — key material does not travel through environment variables.
- The workload, when it is built, is a StatefulSet of one replica with a volume claim template for the agent's volume. The agent's state is a single-writer store, which pins the replica count at 1; the claim template gives a stable volume and ordered replacement.
- `v1alpha1` makes no outbound call. Nothing here reads a source outside the cluster, and no field names one: an identifier for an external API waits until something fills it.
- The reconciler reads the object and returns. It owns no object, holds no finalizer, and writes no status.

## Rationale

The layout and the generated manifests follow the scaffold, per [ADR 0001](adr/0001-kubebuilder-go-v4-scaffold.md): code is written to fit the CLI's paths rather than the CLI's output being reshaped to fit a preferred layout.

The decisions above about the workload's form, the image field, and the absence of outbound calls were settled on issue #11, before the API was written.

## Open questions

- Workload assembly, status writing, and condition semantics. The reconciler is a stub until each is decided.
- The `garam` integration — the endpoint, the credential, and which side opens the connection — and therefore whether it reaches this API in `v1alpha1` or a later version.

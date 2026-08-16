# gagent-operator

A Kubernetes operator that runs `gagent` agent workloads in a cluster from a custom resource.

## What it does

The operator's purpose is to take an agent's desired configuration, hold it as a custom resource, and reconcile the cluster until that agent is running as a Pod. Bringing the configuration in from an external API is the longer-term intent; which API, and how it is reached, is not decided — see `docs/architecture/README.md` and the open issues.

Two things it is not: it is not the agent, which lives in a separate repository, and it is not the source of the configuration.

## Status

Scaffolded, not yet implemented. `PROJECT` lists no resources, and there is no `api/` or `internal/controller/` directory — the first `kubebuilder create api` has not been run. What exists today is the manager entry point, the deployment manifests, the check pipeline, and the rules under `docs/`.

## Requirements

- Go — the version in `go.mod`
- Docker, or another container tool set through `CONTAINER_TOOL`
- `kubectl` and access to a cluster, for the deploy targets
- Kind, for `make test-e2e`

The Makefile downloads controller-gen, kustomize, setup-envtest, and golangci-lint into `bin/` on first use; they are not installed system-wide.

## Running the checks

`make ci` runs the whole check set — lint, format, test, build. It is what CI invokes, and what to run before pushing. `make help` lists every target.

End-to-end tests need a cluster and are not part of that set: `make test-e2e` creates a Kind cluster, runs them, and tears it down.

## Running the operator

Against the cluster in the current kubecontext, with the manager on your machine:

```sh
make install   # apply the CRDs
make run       # run the manager locally
```

Deployed into the cluster instead:

```sh
make docker-build docker-push IMG=<registry>/gagent-operator:<tag>
make deploy IMG=<registry>/gagent-operator:<tag>
```

`make build-installer IMG=<image>` writes a single applyable YAML to `dist/`. No image has been published yet, so `IMG` has no default worth using.

## Rules

Contributing to this repository means following the conventions in it, not general practice.

- `AGENTS.md` — the contribution contract, and who may change what
- `docs/convention/README.md` — the index of every rule that governs code, tests, commits, and documents
- `docs/architecture/README.md` — how the system is shaped now, and the decisions behind it

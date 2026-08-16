# Kubebuilder — Operator Conventions

> How this operator's Go code is laid out and written: directories, API types,
> controllers, markers, logging, tests, and the commands behind the entry-point
> names.
>
> Checked against kubebuilder 4.15.0 (layout `go.kubebuilder.io/v4`, PROJECT
> version 3), controller-runtime 0.24.1, controller-gen 0.21.0, Kubernetes
> client libraries 0.36.0, and Go 1.26. A claim below that names no version
> holds for these.

## Contents
- 0. Folder and file naming
- 1. Directory layout
- 2. Generated code
- 3. API types
- 4. Controllers
- 5. Markers
- 6. Naming
- 7. Error handling
- 8. Logging
- 9. Comments and docs
- 10. Testing
- 11. Imports and dependencies
- 12. Verification commands

## 0. Folder and file naming

The scaffold owns the top-level shape. `cmd/`, `api/`, `internal/`, `config/`,
`test/`, `Dockerfile`, `Makefile`, and `PROJECT` are placed by the kubebuilder
CLI; do not rename or relocate them, because the CLI writes into those exact
paths on the next `create api` or `create webhook`.

Inside a package, a file is named for what it owns: `agent_controller.go`,
`pod_builder.go`, `agent_types.go`.

Banned as authored names, at any level: `model.go`, `utils/`, `helpers/`,
`common/`, `ext/`, `kit/`, `repo/`. If two helpers share a concept, name the
concept.

`test/utils/` is the one exception. It is scaffolded ground, rewritten by the
CLI, and renaming it desynchronizes the scaffold; leave it and do not read it
as licence for a new `utils` elsewhere.

## 1. Directory layout

- `cmd/main.go` — the manager entry point, and the only place that constructs
  concrete controllers and registers them with the manager. Keep it thin: flag
  parsing, scheme registration, manager construction, `SetupWithManager` calls.
- `api/<version>/<kind>_types.go` — one file per kind, holding its `Spec`,
  `Status`, the object, and its list type. This project is single-group, so
  types live directly under `api/<version>/`.
- `api/<version>/groupversion_info.go` — scheme registration for the group
  version. Scaffolded; do not hand-edit.
- `internal/controller/<kind>_controller.go` — one reconciler per kind.
- `internal/controller/suite_test.go` — the envtest bootstrap shared by the
  package's controller tests.
- `internal/<concern>/` — logic that is not a reconciler, named for what it
  owns. A concern moves here from a controller file once it has its own tests
  or a second caller.
- `config/` — kustomize bases and overlays. Partly generated (§2), partly
  hand-edited.
- `test/e2e/` — end-to-end tests, run against a real cluster.
- `dist/` — `make build-installer` output. Ignored, never committed.

A reconciler that outgrows one file splits by responsibility
(`agent_controller.go`, `agent_pod.go`, `agent_status.go`), not by layer.

## 2. Generated code

These are produced by `make manifests generate` and are never hand-edited:

| Path | Produced by |
|---|---|
| `api/<version>/zz_generated.deepcopy.go` | `controller-gen object` |
| `config/crd/bases/` | `controller-gen crd` |
| `config/rbac/role.yaml` | `controller-gen rbac` |
| `config/webhook/manifests.yaml` | `controller-gen webhook` |

Rules:

- A change to an API type or an RBAC marker is committed together with the
  regenerated output, in the same pull request. CI fails when the working tree
  is dirty after `make ci`.
- `// +kubebuilder:scaffold:*` comments are anchors the CLI writes new entries
  into. Never delete, reorder, or move one; a lost anchor makes the next
  `kubebuilder create api` scaffold silently incomplete.
- Editing generated output to fix a problem is the wrong layer. Change the
  marker or the type and regenerate.
- **Upgrade with `kubebuilder alpha update`.** It three-way merges a newer
  scaffold onto this project and reads `cliVersion` from `PROJECT`, so that
  field is kept current. `alpha generate` is not the upgrade path: it
  re-scaffolds in place and deletes everything except `.git` and `PROJECT`.
- **The scaffold carries its own `AGENTS.md`.** kubebuilder writes one into
  every project. This repository's `AGENTS.md` is the contribution contract and
  outranks it. An upgrade that offers to replace that file is refused; its other
  changes are taken normally.

## 3. API types

- **Spec is desired state, Status is observed state.** Never store user intent
  in Status, and never read Status as an input to the decision Reconcile makes.
  A field that both sides write is a defect.
- **Every field carries a doc comment.** It becomes the `description` in the
  CRD and is what a user of the API reads; it is the one comment that is part
  of the published surface.
- **Follow the scaffold's tag style.** `metav1.ObjectMeta` and `Status` take
  `json:",omitzero"`; `Spec` is a non-pointer carrying no omit tag and marked
  `+required`. Optional fields are marked `+optional`, with a pointer where the
  zero value is meaningful. This is not the general Go struct-tag habit, and
  matching it keeps hand-written types indistinguishable from generated ones.
- **Validation belongs in markers** (§5), not in Reconcile, wherever a marker
  can express it. A rejection the API server can make never reaches a
  controller.
- **Status conditions are scaffolded, not added.** The generated `Status`
  already carries `Conditions []metav1.Condition` with `+listType=map` and
  `+listMapKey=type`. Keep those markers — they are what makes the list
  server-side-apply safe — and write the slice only through
  `meta.SetStatusCondition`, never by appending.
- **Re-read the object before updating it again.** A status update bumps the
  resource version, so a second write in the same reconcile using the stale copy
  fails with a conflict.
- **Status carries `observedGeneration`**, set to the object's `Generation` at
  the end of a successful reconcile, so a stale status is detectable.
- An API version is immutable once released. A change that would break an
  existing object is a new version, never an edit.

## 4. Controllers

- **Reconcile is level-triggered and idempotent.** It reads the world as it is
  now and drives it toward Spec. It never depends on which event woke it, how
  many events arrived, or what happened last time.
- **Running Reconcile twice on the same state changes nothing the second time.**
  This is the property every other rule here protects.
- **Never sleep.** To retry later, return `ctrl.Result{RequeueAfter: d}`. A
  blocked worker starves every other object of the same kind. `Requeue: true` is
  deprecated and is not the alternative.
- **`SetupWithManager` names the controller.** The scaffold emits
  `.Named("<kind>")`; keep it. A kind may carry more than one controller
  (`create api --controller-name`, recorded as a `controllers:` array in
  `PROJECT`), and the name is what separates them in logs and metrics.
- **Return the error.** controller-runtime's backoff is the retry mechanism;
  swallowing an error to keep the queue quiet hides a stuck object.
- **A missing object is not an error.** `apierrors.IsNotFound(err)` returns
  `ctrl.Result{}, nil` — the object is gone and there is nothing to reconcile.
- **Owned objects carry an owner reference.** Set it with
  `controllerutil.SetControllerReference` and watch the kind with `Owns()`, so
  deletion cascades and changes wake the owner.
- **Finalizers only for cleanup the garbage collector cannot do** — an external
  resource, something outside the cluster. In-cluster children are cleaned up by
  owner references. A finalizer that is added must be removable on every path,
  or the object cannot be deleted.
- **Status is written through the status subresource** (`.Status().Update` or
  `.Status().Patch`), never through a plain object update.
- **One reconciler per kind.** Two controllers writing one kind's status race.

## 5. Markers

- **RBAC markers sit on the `Reconcile` method** of the controller that needs
  the permission, and name only the verbs it actually calls. `make manifests`
  regenerates `config/rbac/role.yaml` from them; widening the generated role by
  hand is a §2 violation and is reverted on the next generate.
- **A permission a controller does not exercise is removed.** The manager runs
  with the union of every marker in the tree.
- **Emitting an event needs RBAC on `events.k8s.io`, not the core group.** The
  recorder moved to `k8s.io/client-go/tools/events`; a marker naming the core
  group grants nothing.
- **CRD validation markers sit on the field they constrain** — `+kubebuilder:validation:*`
  for bounds and formats, `+kubebuilder:default` for defaults,
  `+kubebuilder:printcolumn` on the type for `kubectl get` output. Cross-field
  constraints belong here too, not in Reconcile: `ExactlyOneOf`, `AtLeastOneOf`,
  `AtMostOneOf`, `+k8s:immutable`, and CEL through `+kubebuilder:validation:XValidation`.
- A marker's effect is verified by reading the regenerated file, not by
  assuming the marker parsed.

## 6. Naming

- **Packages**: single word, lowercase, no underscores.
- **Types**: `MixedCaps`, no underscores. A kind's Go type name matches its
  CRD kind exactly.
- **Functions and methods**: `MixedCaps`, verb-noun.
- **Constants**: `MixedCaps`, grouped in `const ( ... )`. Condition type and
  reason strings are constants, never string literals at the call site.
- **Variables**: short in small scopes, `MixedCaps` at package level.
- **Receivers**: short and consistent across a type's methods (`r *AgentReconciler`).
- **Initialisms**: `URL`, `ID`, `HTTP`, `JSON`, `API`, `TLS`, `CRD` — uniformly
  upper or lower, never mixed.

## 7. Error handling

- Errors are values. Wrap with `fmt.Errorf("op: %w", err)` at boundaries — an
  API call, an external request, a parse — not on every line.
- Sentinels are declared where the concept lives and compared with `errors.Is`.
- Do not log and return the same error. Reconcile returns it; controller-runtime
  logs and requeues.
- A panic in a reconciler takes down the manager. Parse and check rather than
  asserting.

## 8. Logging

- **Use the logger from the context**: the scaffold imports
  `logf "sigs.k8s.io/controller-runtime/pkg/log"` and calls `logf.FromContext(ctx)`.
  Keep the alias. The returned logger carries the controller name, the object's
  namespace and name, and the reconcile ID; constructing a fresh one drops that
  correlation.
- **`logr`, not `log/slog`.** controller-runtime's logging is `logr` end to
  end, and `cmd/main.go` installs a zap-backed implementation.
- **Structured key-value pairs, balanced.** `logcheck` is enabled in
  `.golangci.yml` and fails an odd argument list.
- Levels: `Info` for state changes a user would want to see, `Error` for a
  failure being returned, `V(1)` and deeper for per-reconcile detail.
- **Never log secrets** — token values, key material, or a whole object that
  may carry them. Log the reference, not the content.
- The reconcile path is hot. A log line per reconcile per object is a cost;
  log transitions, not steady state.

## 9. Comments and docs

- Doc comments begin with the name being declared.
- API field comments are published (§3) and are written for the API's user.
- Inline comments earn their place on a non-obvious invariant or an ordering
  constraint — not on paraphrase.
- `// TODO(name): …` with an owner, never `FIXME` or `XXX`.

## 10. Testing

| Layer | Scope | Runs against | Location |
|---|---|---|---|
| Unit | a pure function or builder | nothing | next to the source |
| Controller | one reconciler | envtest — a real API server and etcd, no kubelet | `internal/controller/` |
| E2E | the built image | a real cluster, via Kind | `test/e2e/` |

- **Controller tests use envtest, not a fake client.** A fake client does not
  run defaulting, validation, or the status subresource, so it proves less than
  it appears to.
- **envtest has no kubelet.** Pods created in a controller test stay `Pending`
  forever. A test asserting a Pod reached `Running` belongs in e2e.
- **Ginkgo v2 and Gomega** are the runner and matcher library for controller and
  e2e tests; `ginkgolinter` is enabled in `.golangci.yml`. A pure helper with no
  cluster interaction may use stdlib `testing` directly.
- **E2E tests sit behind `//go:build e2e`.** `go test ./...` does not run them,
  and `make test` excludes the directory as well; only `make test-e2e` does. A
  test that needs a cluster and is not behind the tag will fail every ordinary
  run.
- **Assert on observed state, not on calls.** Read the object back and assert
  its Spec and Status; do not assert that a client method was invoked.
- **Eventual assertions use `Eventually`** with an explicit timeout. Reconcile
  is asynchronous, and a bare read races it.
- **Idempotence is a test case.** Reconcile twice and assert the second call
  changes nothing.
- Test names read like a spec and say what broke without opening the file.

## 11. Imports and dependencies

Three groups, blank-line separated: standard library, third party, this module.
`goimports -local github.com/garamsh/gagent-operator` produces the third group.

- The Kubernetes libraries move together. `k8s.io/api`, `k8s.io/apimachinery`,
  `k8s.io/client-go`, and `sigs.k8s.io/controller-runtime` are upgraded in one
  change, never individually.
- `go mod tidy` after every dependency change, and the result is committed.
- `ENVTEST_VERSION` and `ENVTEST_K8S_VERSION` are derived from `go.mod` by the
  Makefile. Do not pin them by hand; change the module version instead.

## 12. Verification commands

These sit behind the project's entry-point names.

| Task | Name |
|---|---|
| Lint (includes gofmt and goimports checks) | `make lint` |
| Format | `make fmt` |
| Test | `make test` |
| Build | `make build` |
| Whole check set | `make ci` |
| E2E (needs Kind) | `make test-e2e` |
| Regenerate manifests and deepcopy | `make manifests generate` |
| Run against the current kubecontext | `make run` |

`make ci` is what CI invokes. Run it before pushing.

# Curriculum

The full module-by-module spec for this program. This is the map; individual
module directories under `modules/` hold the actual worked material, authored
just-in-time as we reach each one.

Design principles (see also the `k8s-for-ecs-engineers` skill and
[target-roles.md](target-roles.md)):

- **Anchor to ECS/Fargate first**, then explain the real differences.
- **No cloud account is required, ever.** Every exercise runs on the local KIND
  cluster. AWS is the teaching anchor and interview vocabulary, never an
  exercise dependency — see the substitution table in the
  [README](../README.md#environment-constraint-no-cloud-account). When a module
  would reach for a managed AWS service, we self-host the equivalent and name
  what the managed version would have done for us.
- **Production realism** — surface real-world concerns, not just the happy path.
- **K8s development, not just ops** — from M5 on, the emphasis is Go code that
  talks to the Kubernetes API, because that's what the target roles test.
- Every module names the **Teleport challenge level** it feeds, so the whole
  arc converges on a real interview deliverable.

Each module has: learning objectives, the ECS anchor, the hands-on exercise,
the production concerns to surface, and a concrete **Done when** bar.

## The example workload

From M1 on, the thing being deployed is the **[flashcard app](../flashcards/)**
— a small Go + htmx service built for this program — not a stock nginx image.
Deploying something with real config, real state, and real health semantics
makes the production concerns land instead of staying abstract, and each module
adds the layer it just taught:

| Module | What gets added to the app |
|---|---|
| M1 | Deployment + Service, `kind load`, port-forward, rolling update, rollback |
| M2 | Decks from a ConfigMap (`DECKS_DIR`), namespace, least-privilege ServiceAccount |
| M3 | requests/limits, probes wired up, PDB, HPA, PVC for review state, Helm chart, Ingress |
| M4 | `/metrics` via `client_golang`, dashboard, one alert rule, logs in Loki |
| M5 | The same binary starts talking to the Kubernetes API with client-go |

It already splits `/healthz` from `/readyz` and drains on SIGTERM, because
retrofitting those teaches the wrong lesson. Its review state is a plain JSON
file, which fails instructively the first time a Pod reschedules without a
volume — that's the M3 PVC motivation, hit firsthand.

The app is also how the program's **vocabulary drilling** happens: 327
ECS-anchored cards covering M0–M7 and the capstone, scheduled with FSRS. The
first deck is a glossary tier — one term per card — and concept cards declare
their vocabulary with `requires:`, so a concept is never introduced before the
terms it depends on have been retained.

---

## M0 — Local production-shaped cluster

**Objectives:** Install the toolchain (Docker Desktop + KIND + kubectl). Stand
up a **multi-node** KIND cluster (control plane + workers), not a single node.
Understand the control plane vs. worker nodes, and kubeconfig/contexts.

**ECS anchor:** Creating the cluster is like standing up an ECS cluster — but
here the control plane (API server, scheduler, etcd) and the worker nodes are
first-class things you can inspect (`kubectl get nodes`), which ECS hides.
A kubeconfig **context** ≈ an AWS profile + region pointing kubectl at a
specific cluster.

**Exercise:** Create a 3-node cluster (1 control-plane, 2 workers) from a KIND
config file. Inspect nodes, contexts, and the system pods in `kube-system`.

**Production concerns to surface:** Real clusters are multi-node and multi-AZ;
a single-node cluster hides scheduling, disruption, and node-failure behavior.
We deliberately use multiple nodes from day one.

**Study order:** Vocabulary first. `/drill?module=M0` starts with the glossary
terms M0's cards depend on and only unlocks the concept cards once those terms
are retained, so the drill front-loads the ~47 terms M0 uses. Expect the first
few sessions to be almost entirely glossary; that is the intended shape, not a
stall.

**Done when:** `kubectl get nodes` shows one control-plane and two worker nodes
all `Ready`, from a cluster defined in a committed config file.

**Teleport level fed:** tooling foundation for all levels (KIND is exactly what
the challenge recommends).

---

## M1 — Core workloads: Pods, Deployments, Services

**Objectives:** Pod, Deployment (replicas, rolling updates, rollout history),
and Service (ClusterIP / NodePort / LoadBalancer). Internalize the **"Service"
naming collision**.

**ECS anchor:** Task Definition → Pod spec (template). ECS Service → Deployment
(desired replica count + rollouts). The ECS Service's load-balancer/target-group
wiring → a Kubernetes **Service** object (different thing, same word). Task →
Pod. `aws ecs update-service --force-new-deployment` → `kubectl rollout restart`.

**Exercise:** Deploy a small app (start with a stock image; a tiny Go HTTP
server comes later in M5). Scale it up/down, watch a rolling update, roll back
via `kubectl rollout undo`, expose it with a Service, reach it via
`kubectl port-forward`.

**Production concerns to surface:** This is the bare "make it run" baseline —
explicitly note what's still missing (no resource limits, no probes, single
replica) and that those get added in M3. Treat "it's running" like "it worked
in `docker run`": necessary, not sufficient.

**Done when:** App is deployed as a multi-replica Deployment, reachable through
a Service, and I can perform and reverse a rolling update.

**Teleport level fed:** L1–L2 ("deploy a service").

---

## M2 — Config, Secrets, Namespaces, RBAC

**Objectives:** ConfigMap, Secret (and that Secrets are only base64, not
encrypted at rest by default), Namespace, ServiceAccount, and RBAC
(Role/RoleBinding, ClusterRole/ClusterRoleBinding). Least privilege.

**ECS anchor:** Task Definition env vars / Secrets Manager & SSM references →
ConfigMap and Secret objects (now separate API resources you mount/reference).
Task IAM Role → ServiceAccount (+ IRSA on EKS specifically). Namespaces have
**no real ECS equivalent** — closest is "use separate clusters."

**Exercise:** Move the M1 app's config into a ConfigMap + Secret. Create a
dedicated Namespace. Create a ServiceAccount bound to a **least-privilege Role**
that can only `get`/`list`/`watch` (and later `patch`) Deployments — this is
exactly the permission set the capstone server will need.

**Production concerns to surface:** Plain Secrets need a real backing store in
production. **We use HashiCorp Vault (self-hosted, in-cluster) as the external
store and External Secrets Operator (ESO) as the bridge** — no AWS account
needed, and it's the most portable/employable of the options. Cover the
alternatives conceptually so I can compare them in an interview: ESO + AWS
Secrets Manager/SSM (the managed-cloud version of exactly what we're building),
the Secrets Store CSI Driver (never lands in etcd), and the Git-native options
(Sealed Secrets, SOPS). Also surface: rotation doesn't restart Pods, env-var
Secrets leak more than mounted files, and RBAC on `get secrets` is the real
access control.

**Vault-specific angle worth the time:** wire Vault's **Kubernetes auth method**,
where Vault validates a Pod's ServiceAccount token via the TokenReview API and
issues a short-lived Vault token. That's the self-hosted mirror of IRSA / EKS
Pod Identity — workload identity with no static credential anywhere — and it
connects directly to the ServiceAccount work in this module. Stretch goal:
Vault **dynamic secrets** against an in-cluster Postgres, so credentials are
minted per-workload with a TTL and expire on their own. That's the "the best
secret is no secret" idea made concrete, and it's a strong interview story.

**Done when:** App reads config from ConfigMap/Secret in its own namespace; the
Secret is populated by ESO from Vault (nothing secret is committed to the repo);
and there's a ServiceAccount whose Role grants only the Deployment permissions
it needs (verified with `kubectl auth can-i`).

**Teleport level fed:** L3 (ServiceAccount) and L5 (Role/RoleBinding in the
production Helm packaging).

---

## M3 — Production concerns

**Objectives:** `resources.requests`/`limits`; liveness/readiness/startup
probes; HorizontalPodAutoscaler; PodDisruptionBudget; NetworkPolicy; **Helm**
(chart authoring, values, upgrades); Ingress + an ingress controller.

**ECS anchor:** Task CPU/memory → requests/limits (but split into
scheduling-guarantee vs hard-ceiling). ECS/ALB health checks → probes (which
also gate rollouts and restarts, not just LB traffic). Service auto scaling →
HPA (pod count) — node autoscaling is a separate layer. ALB + target group →
Ingress + controller. PodDisruptionBudget has no direct ECS analog but maps to
"don't let a capacity rebalance take my service down."

**Exercise:** Harden the app — add requests/limits, readiness + liveness
probes, an HPA, and a PodDisruptionBudget. Package the whole thing as a **Helm
chart** (Deployment, Service, ServiceAccount at minimum) and verify a
**zero-downtime `helm upgrade`**. Add an Ingress. Optionally add a
NetworkPolicy to default-deny and then allow only needed traffic.

**Production concerns to surface:** All of the above are the difference between
a demo and a service that survives node drains, cluster upgrades, and traffic
spikes. A Deployment without readiness probes routes traffic to not-ready pods —
a real bug class.

**Done when:** The app is deployed **via a Helm chart** with limits, probes,
HPA, and a PDB, and a `helm upgrade` completes without dropping traffic.

**Teleport level fed:** L3 (Helm chart with Deployment/ServiceAccount/Service;
upgrades must not cause unavailability).

---

## M4 — Observability

**Objectives:** Prometheus, Grafana, Loki (all three named explicitly in the
SRE job description). Metrics vs. logs vs. traces. `kubectl logs`/`kubectl top`
as debugging tools, not a monitoring strategy. Instrumenting an app with a
Prometheus `/metrics` endpoint.

**ECS anchor:** CloudWatch Container Insights + the `awslogs` log driver →
**nothing ships out of the box**; you self-host the stack (Prometheus/Grafana
for metrics, Loki/Fluent Bit for logs). This is a genuine gap vs. ECS, not just
renamed tooling.

**Exercise:** Install `kube-prometheus-stack` via Helm. Explore cluster and app
metrics in Grafana. Add Loki for log aggregation. Instrument a small Go app with
the Prometheus client library exposing `/metrics`, scrape it, and build/import a
dashboard plus one alert rule.

**Production concerns to surface:** Observability is an owned responsibility in
K8s. Alerting with low false-positive rates (called out in the SRE JD) matters
for on-call sanity.

**Done when:** Grafana shows both cluster metrics and a custom app metric from a
Go `/metrics` endpoint, logs are queryable in Loki, and there's one working
alert rule.

**Teleport level fed:** L3 health checks; directly matches the SRE role's
Prometheus/Grafana/Loki requirement and its observability responsibilities.

---

## M5 — Kubernetes development in Go (client-go)

**Objectives:** Go module setup; `client-go`; in-cluster vs. out-of-cluster
config; talking to the Kubernetes API; the **informer/lister/watch** pattern and
a local cache; the reconciliation mental model. This is where the program pivots
from operating clusters to **developing against them** — the differentiator both
target roles care about.

**ECS anchor:** ECS APIs are mostly imperative one-shot calls (call it, it acts
once). Here you write a **long-running client that watches** the API and reacts
to changes continuously — a different operating model.

**Exercise (this begins the capstone):** A Go HTTP server that:
1. reads a Deployment's replica count (Teleport **L1**),
2. sets the replica count and lists Deployments (**L2**/**L3**),
3. adds a health check that verifies real Kubernetes API connectivity (**L3**),
4. replaces per-request API calls with a **watch-based informer cache** so reads
   don't hit the API server every time (**L4** caching).
With happy-path and unhappy-path tests, run against the local KIND cluster.

**Production concerns to surface:** Don't poll the API server per request
(informer cache). Handle API errors without crashing. Use the least-privilege
ServiceAccount from M2. Config via in-cluster service account token when
deployed, kubeconfig when local.

**Done when:** A tested Go server, deployed to KIND via its Helm chart, serves
replica-count read/set + Deployment list from an informer-backed cache, with a
health check that fails when the API is unreachable.

**Teleport level fed:** L1–L4 server portions.

---

## M6 — CRDs + controllers (the operator pattern)

**Objectives:** CustomResourceDefinitions; `controller-runtime` /
`kubebuilder`; the reconcile loop; desired-vs-actual state stored in a CR;
status subresource; finalizers. This is the extensibility model at the heart of
Kubernetes.

**ECS anchor:** **Nothing in ECS resembles this.** Don't force an analogy —
this is genuinely new. The closest conceptual hook is the reconciliation loop
itself (declared desired state in etcd, a controller continuously converging
actual state toward it), which underlies all of Kubernetes.

**Exercise:** Define a CRD (e.g. a per-Deployment desired replica count) and
write a controller that **reconciles** the target Deployment to match the CR's
spec — a real, small operator. Include status reporting and handle conflicts.

**Production concerns to surface:** Idempotent reconciliation, requeue on
error, handling concurrent changes/conflicts, not fighting other controllers.

**Done when:** Creating/editing a CR causes the controller to drive the target
Deployment's replica count to the desired value and report status, verified on
KIND with tests.

**Teleport level fed:** L5.

---

## M7 — Cluster authentication: identity & short-lived credentials

**Deliberate revisit.** M2 *uses* ServiceAccount tokens as a tool (Vault's
Kubernetes auth method exchanges one for a Vault token) without opening them up.
This module opens them up. The repetition is intentional: use the tooling early
to get something working, then come back and understand the mechanism. Nothing
in the capstone depends on this module, so it can slot late or be skipped if
time runs short.

**Objectives:** How the API server decides *who you are* — as distinct from what
you may do, which is RBAC from M2. The three credential types: the X.509 client
cert KIND hands you by default, ServiceAccount tokens via the TokenRequest API,
and OIDC as the way real clusters authenticate humans. The through-line is
**credential lifetime**: moving from a long-lived unrevocable cert to
short-lived, refreshable tokens.

**ECS anchor:** Partial at best, and the gap is the lesson. In ECS, IAM is the
whole authorization story — an IAM principal with the right policy calls the ECS
API, full stop. Kubernetes stacks **two independent systems**: something external
authenticates you (a cert, an OIDC token, an IAM principal on EKS), then
Kubernetes RBAC separately decides what you may do. The genuinely new idea is
that **Kubernetes has no User object** — usernames are strings asserted per
request by the authn layer and never stored, which is why there's no
`kubectl create user`, and why human identity always comes from outside.

**Exercise:**
1. Inspect the KIND admin credential — decode the client cert out of the
   kubeconfig, read its `CN`/`O`, and connect that to the pre-created
   `kubeadm:cluster-admins` ClusterRoleBinding. Confirm with
   `kubectl auth whoami` and `kubectl auth can-i --list`.
2. Open up the M2 ServiceAccount token: mint one with `kubectl create token <sa>
   --duration=10m`, decode its claims, use it as a credential directly, and
   watch it expire. Then trace how Vault's Kubernetes auth method validated that
   same kind of token via the **TokenReview** API — the piece M2 took on faith.
3. *(stretch)* Wire the cluster to a real OIDC provider — apiserver side via
   `kubeadmConfigPatches`, client side via the `kubelogin` credential plugin —
   and bind RBAC to the resulting identity. **Keep the cert-based context as
   break-glass**; an OIDC-only KIND cluster with a bad config locks you out with
   no console fallback.

**Production concerns to surface:** The KIND admin cert is cluster-admin,
effectively unrevocable (no CRL in play — the only remedy is rotating the cluster
CA) and never expires in practice. Fine for a throwaway cluster, and exactly why
nobody uses client certs for humans in production. Short-lived tokens are the
norm, and it's the same mechanism behind IRSA / EKS Pod Identity and anything a
CI system uses. Also: identity providers carrying no `groups` claim (Google's ID
tokens, for one) force per-user RBAC bindings, defeating the point of RBAC's
group indirection — hence Dex or a similar federator in front.

**Version notes** (verified 2026-07-31 against KIND v0.32 / node image v1.36.1;
re-check before authoring, this area moves):
- Structured authentication configuration (`--authentication-config` +
  `AuthenticationConfiguration`) went **stable in v1.34** and is the
  production-recommended path — multiple issuers, CEL claim mapping, reloadable
  without an apiserver restart.
- The `--oidc-*` apiserver flags still exist and are **not** marked deprecated in
  source. Several blog posts claim they were removed in v1.35 — that is
  **false**; don't lose time to it. The flags are the simpler starting point on
  KIND (just a `kubeadmConfigPatches` block); the config file needs an
  `extraMounts` to reach the control-plane container.
- The in-tree kubeconfig `auth-provider: oidc` was **not** removed (unlike the
  `gcp`/`azure` providers, which were, in v1.26). `kubelogin` is still the right
  tool, because it performs the initial browser login rather than only refreshing
  a token you already hold.
- KIND v0.32 uses **kubeadm v1beta4** for K8s v1.36+, where `extraArgs` is a
  **list of `name`/`value` pairs**, not a map. Most tutorials online still show
  the map form, which silently fails to apply.

**Done when:** I can explain without notes how a `kubectl` request is
authenticated on KIND vs. on EKS, and I've authenticated to the cluster with a
credential that expires.

**Teleport level fed:** none directly — this is interview-and-incident depth. It
deepens the M2 ServiceAccount/RBAC work and the "in-cluster service account token
vs. kubeconfig" config path used in M5.

---

## Capstone — The Teleport SRE take-home, leveled 1 → 5

**Objectives:** Do the real challenge end to end as a rehearsal and portfolio
piece: an **RFD-style design doc first** (API structure, pod lifecycle, TLS
config, developer workflow), then a leveled implementation culminating in a
**gRPC API secured with mTLS**, a **CRD + reconciling controller**, and
**production-grade Helm packaging** (Deployment, Role, RoleBinding,
ServiceAccount, Service), all `make`-driven and KIND-deployable, with tests for
happy and unhappy paths.

By this point M5 and M6 have already built most of the server and controller;
the capstone is about assembling it to the challenge's bar, adding gRPC + mTLS,
writing the design doc, and practicing the design-doc-first, small-scoped,
well-tested workflow the challenge rewards.

**Explicit non-goals** (per the challenge's own guidance — don't over-build):
no shared cache, no multi-region/multi-AZ, no gold-plated config system.
Hardcode and leave `TODO:` comments to show thinking. Cut scope, not quality.

**Important:** Teleport's challenge says **not to outsource the thinking to AI**.
This capstone is a rehearsal to build the skill and a portfolio artifact — the
**actual interview submission must be written by me**.

**Teleport level fed:** L1–L5 — the whole thing.

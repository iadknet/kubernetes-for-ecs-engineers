---
name: k8s-for-ecs-engineers
description: Use whenever explaining, teaching, or discussing any Kubernetes concept, resource, command, or workflow in this project, and whenever building or reviewing any hands-on exercise, manifest, or PoC. Frames every explanation in terms of AWS ECS/Fargate equivalents (or explicitly flags where none exists), and steers all hands-on work toward realistic production configurations and concerns rather than toy/local setups, so someone with deep ECS/Fargate experience and zero K8s experience builds job-interview-ready understanding.
---

# Kubernetes for ECS/Fargate Engineers

The user has extensive hands-on AWS ECS/Fargate experience and no Kubernetes
experience. They are learning Kubernetes to close a gap in their job search.
Every explanation of a Kubernetes concept should anchor to what they already
know from ECS/Fargate rather than teaching K8s in a vacuum. All hands-on
training work should target production-grade realism, not just "make it run
locally" — see "Train for production, not just for green checkmarks" below.

This project is loosely grounded in real job postings — see
[`docs/target-roles.md`](../../../docs/target-roles.md) for the current
target roles (Teleport Senior SRE and IT Security & Automation Engineer)
and their take-home coding challenges. Use them as a reference for a
realistic bar and relevant topics, but per the "Scope" section below don't
over-optimize for these two postings — transferable core Kubernetes skill is
the goal. One thing worth carrying forward from them: modern SRE/DevOps work
(and both roles) involves Kubernetes *development*, not just operations — Go
code against the Kubernetes API (client-go/controller-runtime, CRDs/operators,
Helm), so include real API-level Go work, not only running manifests. When an
exercise maps to the SRE challenge (a Deployment-replica API, a Helm chart, a
controller/CRD), say so and hold it to that challenge's bar: design-doc-first,
tests for happy and unhappy paths, avoid scope creep.

## Hard constraint: AWS is the anchor, never the tooling

**The user has no AWS account.** As of 2026-07-31 they cannot run anything that
requires AWS credentials, an AWS account, or AWS-hosted services. Their ECS/
Fargate *experience* is undiminished and remains the anchor for every
explanation — but the split is now absolute:

- **Explanations, analogies, and interview prep: AWS stays central.** Keep using
  ECS/Fargate as the mapping source, and keep teaching EKS/IRSA/Secrets Manager/
  CloudWatch as knowledge — the user still has to speak to all of it fluently in
  interviews. Nothing in the mapping table or the "Running on EKS" section is
  retired.
- **Exercises, commands, and manifests: no AWS, ever.** Never author an exercise,
  example command, or manifest that needs an account. No `aws` CLI as a required
  step, no `eksctl`, no IRSA setup, no ECR push, no Secrets Manager reference.
  If an exercise can't run on the local KIND cluster, it isn't an exercise —
  demote it to an explanation.

When a topic would naturally reach for a managed AWS service, **substitute the
self-hosted equivalent and explicitly name what the managed version would have
done for you.** That contrast is itself good teaching, and it's how the user
stays interview-ready on AWS without touching it. Current substitutions (the
authoritative table lives in [`README.md`](../../../README.md)):

| Production on AWS | Use instead (local, on KIND) |
|---|---|
| Secrets Manager / SSM Parameter Store | **HashiCorp Vault** in-cluster + **External Secrets Operator** |
| IRSA / EKS Pod Identity | **Vault Kubernetes auth** (ServiceAccount token → TokenReview) |
| ALB + AWS Load Balancer Controller | **ingress-nginx** |
| CloudWatch Container Insights / `awslogs` | **Prometheus + Grafana + Loki** |
| ECR | local images via `kind load docker-image` |
| EKS managed control plane | KIND (1 control-plane + 2 workers) |

This *reinforces* the portability goal in "Scope" below — self-hosting the
equivalent forces learning the mechanism rather than the AWS console button.

## Scope: workload SRE/DevOps, not cluster/platform operator

The user's target is being a **Kubernetes-using SRE/DevOps engineer** — someone
who deploys, operates, debugs, and hardens *workloads* on a cluster whose
control plane and nodes are kept healthy by a managed service (EKS/GKE/AKS) or
a separate platform team. They explicitly do **not** want to specialize in the
underlying-infrastructure / cluster-operator role: bare metal, VM/hypervisor,
node provisioning and AMI/OS patching, control-plane lifecycle, and cluster
build-out are *not* the career target. Weight teaching accordingly:

- **Primary (go deep, hold to a production bar):** the workload/consumer layer
  — Deployments/StatefulSets, Services/Ingress, config/secrets, RBAC,
  NetworkPolicy, probes, resource requests/limits, HPA, PodDisruptionBudgets,
  topology spread, rollouts/rollbacks, Helm, GitOps, observability of services,
  and Go code against the Kubernetes API.
- **Secondary (understand conceptually, don't specialize):** the node/infra
  layer — node groups, Karpenter/Cluster Autoscaler internals, AMI patching,
  control-plane HA, cluster version upgrades, CNI/CSI plumbing. Teach these at
  the level of *"how does this layer affect my workloads, and what do I need to
  know to reason about it in an incident or an interview"* — not as things the
  user will own day to day. Frame them as "platform-team / managed-service
  territory" and keep the depth proportional to that.

Also: don't over-index on AWS/EKS. Core Kubernetes is portable and that
portability is the point — the user wants transferable SRE/DevOps skills across
managed providers. Use EKS as the concrete example (it matches their AWS
background and the Teleport roles), but keep portable concepts primary and flag
when something is an AWS-specific detail rather than core Kubernetes.

## How to use this skill

1. When introducing a K8s concept, lead with "this is like [ECS/Fargate thing]"
   using the mapping table below, then explain how it actually differs.
2. Never let an analogy stand unqualified if it's misleading — say what breaks
   if you push the analogy too far (see "Where the analogy breaks down").
3. When a K8s concept has **no real ECS/Fargate equivalent**, say so explicitly
   instead of forcing a strained comparison. These are the concepts worth the
   user spending extra attention on, since prior experience won't shortcut them.
4. Prefer concrete command/manifest comparisons (`kubectl` vs `aws ecs` CLI,
   YAML vs task definition JSON) over abstract description when practical.
5. This applies to explanations, code comments in training material, README
   content, and any conversational answer — not just formal docs.
6. Give the EKS lens when it affects the *workload* layer, not reflexively.
   The user runs on managed control planes (EKS matches their background), so
   surface "here's the vanilla/kind behavior, here's what changes on EKS" when
   it changes how they deploy or operate a service — see "Running on EKS"
   below. For the node/infra-operator parts of that section, keep it to
   conceptual awareness per the "Scope" section above rather than deep dives.
   Always label which layer you're describing so the user can tell managed
   AWS behavior apart from portable Kubernetes behavior.

## Core concept mapping

| ECS/Fargate | Kubernetes | Notes |
|---|---|---|
| Cluster | Cluster | Similar idea, but a K8s cluster always includes the control plane (API server, scheduler, etcd) as a first-class concept you can inspect — there's no ECS equivalent to `kubectl get nodes` showing control-plane-managed workers. |
| Task Definition | Pod spec (usually inside a Deployment/StatefulSet manifest) | A Task Definition is closest to a Pod template. But K8s adds a layer on top (Deployment) that ECS's "Service" partially covers — see below. |
| Task | Pod | A Pod is the smallest deployable unit, like a Task. Pods almost always hold one container, but *can* hold sidecars — similar to multi-container Task Definitions. |
| Container (within a Task) | Container (within a Pod) | Near 1:1. |
| ECS Service | Deployment (+ the Deployment controller) | A Deployment manages desired replica count and rolling updates, like an ECS Service. Key difference: Deployment doesn't do networking/load-balancer wiring itself — that's a Service (see next row, and yes, the name collision with "ECS Service" is a common early confusion point). |
| ECS Service's load-balancer target group wiring | Kubernetes Service (type `ClusterIP`/`NodePort`/`LoadBalancer`) | Confusingly, "Service" in K8s means something closer to ECS's *combination* of a Service's networking + target group, not the ECS Service resource itself. This naming collision trips up almost everyone coming from ECS — call it out explicitly when relevant. |
| Fargate (serverless compute for tasks) | No direct built-in equivalent; closest are Fargate profiles on EKS, or node-based autoscaling (Cluster Autoscaler / Karpenter) | Vanilla K8s assumes you manage worker nodes (EC2, on-prem, etc.). EKS Fargate exists and maps closely, but "Fargate-like serverless-by-default" is not the baseline K8s mental model — nodes are usually a first-class thing you think about. |
| Task Definition CPU/memory | Pod `resources.requests` / `resources.limits` | Conceptually similar (reserve vs cap), but K8s separates *requests* (scheduling guarantee) from *limits* (hard ceiling) more explicitly than ECS's single CPU/memory value. |
| ECS Cluster capacity (EC2 or Fargate) | Nodes (worker machines, each running a `kubelet`) | If not using EKS Fargate, you are responsible for node capacity/scaling the way you'd manage an EC2-backed ECS cluster, not a Fargate one. |
| Application Load Balancer + Target Group | Ingress (+ an Ingress Controller like ALB Ingress Controller, NGINX, etc.) | K8s Ingress is a spec; you still need a controller to actually provision the load balancer — on EKS that's commonly the AWS Load Balancer Controller, which *does* create a real ALB/NLB under the hood. |
| Service Discovery (AWS Cloud Map / ECS Service Connect) | Kubernetes DNS (CoreDNS) + Services | K8s Services get a stable DNS name (`myservice.namespace.svc.cluster.local`) automatically — this is closer to Service Connect than to Cloud Map's opt-in model. |
| Task IAM Role | Kubernetes ServiceAccount + IRSA (IAM Roles for Service Accounts, on EKS) | Direct conceptual parallel on EKS specifically. Vanilla K8s ServiceAccounts have no AWS IAM concept at all without IRSA wired up. |
| ECS Task placement / capacity providers | Scheduler + node selectors/affinity/taints-tolerations | K8s exposes much finer-grained placement control than ECS placement strategies/constraints. |
| CloudWatch Logs (`awslogs` driver) | No built-in equivalent — logs go to stdout/stderr on the node and you bring your own aggregation (Fluent Bit, CloudWatch Container Insights, etc.) | This is a real gap, not just naming — K8s doesn't ship a managed logging pipeline the way ECS's `awslogs` driver does out of the box. |
| ECS Exec | `kubectl exec` | Near 1:1. |
| Auto Scaling (Service auto scaling / Target Tracking) | Horizontal Pod Autoscaler (HPA), Cluster Autoscaler/Karpenter for nodes | K8s splits this into two layers explicitly: scaling *pod count* (HPA, like ECS Service auto scaling) vs scaling *node count* (separate concern, closer to ECS Capacity Providers with EC2). |
| `aws ecs update-service --force-new-deployment` | `kubectl rollout restart deployment/<name>` | Similar operational intent. |
| Task Definition revisions | Deployment revision history (`kubectl rollout history`) | Similar rollback capability. |
| No strong ECS equivalent | Namespace | A soft multi-tenancy/scoping boundary within a cluster. ECS clusters don't subdivide this way — the closest ECS gets is "just use separate clusters." |
| No ECS equivalent | ConfigMap / Secret as first-class API objects | ECS handles this via task definition environment variables or Secrets Manager/SSM references. K8s ConfigMaps/Secrets are separate API resources you create and then mount/reference — more moving parts, more flexibility. |
| No ECS equivalent | Custom Resource Definitions (CRDs) / Operators | Kubernetes' plugin/extension model — nothing in ECS resembles this. This is a genuinely new concept, not a renamed familiar one. |
| No ECS equivalent | `kubectl` imperative vs declarative apply model, and the reconciliation loop pattern generally | ECS APIs are mostly imperative (call an API, it does the thing). K8s controllers continuously reconcile actual state toward declared desired state in etcd. This reconciliation-loop mental model underlies almost everything else in K8s and is worth understanding deliberately rather than by analogy. |

## Where the analogy breaks down (call these out explicitly)

- **"Service" means two different things.** ECS Service ≈ K8s Deployment (for
  scaling/rollout), but K8s *also* has a resource literally named "Service"
  that's about networking, not replica management. Always disambiguate.
- **Declarative reconciliation, not imperative API calls.** `kubectl apply -f`
  writes desired state to etcd; controllers converge actual state to it
  continuously. This is a different operating model from calling
  `aws ecs update-service` and having it execute once.
- **Nodes are a visible, first-class concept (even where you don't own them).**
  Unlike Fargate's serverless-by-default model, K8s always exposes nodes — Pods
  land on them and node capacity/health directly affects your workloads. Who
  *manages* node lifecycle (sizing, AMIs, patching, autoscaling) varies: on
  self-managed K8s it's on you; on EKS/managed clusters it's the platform's (see
  "Scope"). The mindset shift from Fargate is that nodes exist and you reason
  about them in incidents, not necessarily that you operate them.
- **YAML sprawl and the layering of objects.** A single "deploy a service"
  outcome in ECS is one Task Definition + one Service. In K8s it's commonly
  a Deployment + a Service + maybe an Ingress + a ConfigMap + a Secret +
  a ServiceAccount — several separate objects that compose, versus one or
  two ECS resources with more built-in fields.
- **The extensibility model (CRDs/Operators) has no ECS parallel at all** —
  don't force an analogy here, just teach it as new.

## Train for production, not just for green checkmarks

Local proof-of-concepts (kind/minikube, `kubectl apply` with defaults, a
single-replica Deployment with no resource limits) are fine as a first pass
to see a mechanism work. They are not the destination. Whenever setting up
an exercise, writing a manifest, or reviewing the user's work, actively push
toward what the same thing looks like run for real, and call out the gap
when a PoC skips it. Treat "it worked in kind" the way you'd treat "it
worked in `docker run` on my laptop" for ECS — a necessary but insufficient
bar.

Concretely, default to surfacing these production concerns rather than
waiting to be asked:

- **Resource requests/limits are mandatory, not optional.** A Pod with no
  `resources.requests`/`limits` is the K8s equivalent of an ECS Task
  Definition with no CPU/memory reservation — it "works" in a demo and
  causes noisy-neighbor or OOM incidents in a real cluster. Call this out
  any time an example omits it.
- **Health checks matter more than in Fargate.** Fargate/ECS health checks
  gate ALB traffic; K8s liveness/readiness/startup probes also gate
  rollouts, restarts, and traffic within the mesh/Service. A Deployment
  without readiness probes will happily route traffic to a Pod that isn't
  ready yet — flag this as a real production bug class, not a nitpick.
- **Namespaces, RBAC, and NetworkPolicies are the multi-tenant safety net.**
  A single-namespace, cluster-admin, no-NetworkPolicy setup is fine for a
  10-minute demo and a real problem in any shared cluster. When an exercise
  skips these, say so and, where reasonable, add a minimal version rather
  than silently leaving it out.
- **Secrets need a real backing store.** Plain K8s `Secret` objects are only
  base64-encoded, not encrypted at rest by default. Mention external-secret
  patterns (External Secrets Operator + AWS Secrets Manager/SSM, Sealed
  Secrets, SOPS) as the production analog to how ECS Task Definitions
  reference Secrets Manager directly.
- **Rollout strategy and PodDisruptionBudgets.** A bare Deployment defaults
  to a rolling update, but without a `PodDisruptionBudget` and sane
  `maxUnavailable`/`maxSurge`, node drains and cluster upgrades can take
  down a service the way an unplanned ECS capacity rebalance would. Bring
  this up around anything involving scaling, node upgrades, or cluster
  maintenance.
- **Autoscaling has two layers; own the pod layer, understand the node layer.**
  Go deep on pod count (HPA) and right-sized requests/limits informed by real
  usage — that's the user's workload responsibility. Node-level autoscaling
  (Karpenter/Cluster Autoscaler) is mostly platform/managed-service territory:
  know *that* it exists and how it reacts to unschedulable Pods (so they can
  debug "my Pod is Pending"), without going deep on operating it.
- **Observability is not built in.** Unlike ECS's `awslogs` driver and
  CloudWatch Container Insights, K8s ships nothing for logs/metrics/traces
  out of the box. Production exercises should at least gesture at the real
  stack (e.g., Fluent Bit/Fluentd + CloudWatch or a log aggregator,
  Prometheus/Grafana or CloudWatch Container Insights for EKS, and how
  `kubectl logs`/`kubectl top` are debugging tools, not a monitoring
  strategy).
- **Multi-AZ and node failure are first-class scenarios — from the workload
  side.** ECS/Fargate spreads tasks across AZs mostly for you. In K8s, the
  *workload-level* controls the user owns — topology spread constraints, pod
  anti-affinity, PodDisruptionBudgets — are what keep their service up when a
  node or AZ dies. (Provisioning multi-AZ node groups is the platform layer;
  they consume it, they don't build it.) Don't let an exercise implicitly
  assume a single healthy node.
- **Cluster upgrades: know the workload blast radius, not the upgrade runbook.**
  Managed control-plane and node-AMI upgrades are largely handled for the user
  on EKS/GKE/AKS. What they own is the fallout: deprecated API versions
  breaking their manifests on a version bump, and whether their PDBs/probes let
  the service ride out a node roll. Teach it as "how do I make my workload
  survive an upgrade I don't control," not as an operator's upgrade procedure.
- **CI/CD and GitOps.** Manually `kubectl apply`-ing from a laptop is fine
  for learning a mechanism, but production K8s workflows are typically
  GitOps-driven (Argo CD/Flux) or pipeline-driven, closer in spirit to how
  CDK/CloudFormation/Terraform manage ECS — mention this distinction when
  relevant so manual `kubectl apply` habits don't look production-ready.

When building an exercise or reviewing the user's manifests/YAML, don't
silently "fix" these gaps and move on — name the gap, explain the production
risk it maps to (ideally via an ECS/Fargate-flavored incident scenario the
user would recognize), and then show or suggest the production-grade
version. The goal is that the user comes away able to say, in an interview,
not just "I got a Pod running" but "here's what I'd also need for this to be
production-safe, and why."

## Running on EKS: the AWS-managed layer (what changes vs. vanilla/kind)

Everything above is portable Kubernetes; this section is the AWS-specific
layer. **This section is knowledge, not tooling** — per "Hard constraint" above,
the user has no AWS account, so none of it may become a hands-on step. It stays
because EKS is where they're most likely to land and it comes up constantly in
interviews. Lead with the core concept and treat EKS as the concrete instance,
not the point. Mental frame that maps to their ECS
experience: **EKS is to a self-managed K8s cluster roughly what managed
ECS/Fargate is to a self-managed Docker host** — AWS takes over some layers, but
*fewer than Fargate did*, and the seams are where the work lives.

Items are tagged **[workload]** (theirs to know deeply) or **[platform]**
(managed-service / platform-team territory — conceptual awareness for incidents
and interviews, not a specialization). Split each along "what AWS now manages
for you" vs. "what's newly your job because it's Kubernetes on AWS":

- **[platform] The control plane is managed (like ECS's hidden control plane).** On EKS,
  AWS runs the API server, etcd, scheduler, and controller-manager across AZs
  for you — you never SSH to a control-plane node the way this kind cluster
  lets you. This is the one place EKS feels like ECS/Fargate: the brain is
  AWS's problem. You pay per cluster-hour for it.
- **[platform] Node lifecycle (mostly not the user's target).** Classic EKS =
  managed node groups (EC2 Auto Scaling groups of worker nodes) or self-managed
  nodes. You own AMI selection, node IAM roles, patching cadence, and node
  autoscaling (**Karpenter** is the current standard; Cluster Autoscaler is the
  older path). **EKS Fargate profiles** remove node management for matched
  Pods and are the closest thing to the Fargate model they know — but come with
  real limits (no DaemonSets, no privileged Pods, per-Pod sizing). **EKS Auto
  Mode** (newer) manages nodes and core add-ons for you and is worth mentioning
  as the "most Fargate-like full-cluster" option.
- **[workload] Networking is real VPC networking via the AWS VPC CNI.** Familiar
  from ECS `awsvpc` mode: EKS Pods get **real VPC IPs** from your subnets (the
  `amazon-vpc-cni-k8s` plugin), so Pod density per node is capped by ENI/IP
  limits and you size subnets for Pod count, not just node count — the
  IP-exhaustion failure mode is sharper than on ECS. Security Groups for Pods
  and NetworkPolicy support are add-on concerns.
- **[workload] IAM ↔ Kubernetes identity is a two-way bridge to learn.** Two directions,
  both new:
  - *Pods → AWS*: **IRSA** (IAM Roles for Service Accounts) or the newer **EKS
    Pod Identity** are how a Pod assumes an AWS IAM role — the direct analog of
    an ECS **Task Role**. Prefer teaching Pod Identity as the current default,
    IRSA as the widely-deployed incumbent.
  - *Humans/AWS → cluster*: **access entries** (or the legacy `aws-auth`
    ConfigMap) map IAM principals to Kubernetes RBAC. There is no ECS analog —
    ECS authorization is just IAM. Here IAM gets you to the API server, then
    Kubernetes RBAC decides what you can do. Call out that these are two
    distinct authz systems stacked.
- **[workload] Load balancing is provisioned by the AWS Load Balancer Controller.** An
  `Ingress` becomes a real **ALB**, and a `Service type=LoadBalancer` (with the
  right annotations) becomes an **NLB**. Without that controller installed,
  those objects don't wire up to anything. Annotations are where most of the
  real config lives — this is the closest mapping to ECS Service → target group.
- **[workload] Storage is EBS/EFS via CSI drivers.** `PersistentVolumeClaims` bind to EBS
  volumes (`ebs-csi-driver`) or EFS (`efs-csi-driver`), installed as add-ons.
  EBS is single-AZ — a Pod using an EBS PVC is pinned to that volume's AZ,
  which interacts with scheduling and is a classic multi-AZ gotcha.
- **[workload] Observability wires into CloudWatch (but you still install it).** Container
  Insights, Fluent Bit → CloudWatch Logs, and managed Prometheus/Grafana (AMP/
  AMG) are the AWS-native stack — closer to what they know than raw
  Prometheus, but unlike ECS's `awslogs` driver, **none of it is on by
  default**; it's add-ons you deploy.
- **[platform] Cluster version upgrades are a recurring operational reality.** AWS supports each
  K8s minor version for a limited window then force-upgrades; someone owns
  upgrading the control plane, then node groups, then add-ons, watching version
  skew and deprecated APIs. On managed clusters this is largely a platform-team
  concern — but the *workload* fallout the user does own is real: deprecated
  API versions breaking their manifests, and PodDisruptionBudgets / probes
  determining whether their service survives the node roll. Teach it from that
  angle.
- **[platform] Provisioning is IaC, same instinct as ECS.** `eksctl`, Terraform, or the
  CDK/CloudFormation define the cluster — the same "don't click in the console,
  declare it" habit they already have for ECS. `aws eks update-kubeconfig`
  is how the local `kubectl` gets credentials, analogous to configuring the
  AWS CLI for a cluster.

When teaching a concept on kind, add a one-line "on EKS this becomes…" note
wherever the managed-service behavior differs materially, so the user is never
surprised by the gap between what works in this training cluster and what an
interviewer means by "in production."

## Style guidance

- Keep comparisons honest: a wrong or oversimplified analogy is worse than
  saying "this is genuinely new, here's why."
- When giving `kubectl` examples, show the ECS CLI equivalent alongside it
  where one reasonably exists.
- Always label portable-Kubernetes behavior vs. EKS-specific behavior (see
  "Running on EKS") — in interviews both come up, and the user wants to keep
  the two straight.

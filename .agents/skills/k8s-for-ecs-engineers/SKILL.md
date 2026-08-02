---
name: k8s-for-ecs-engineers
description: Use whenever explaining, teaching, or discussing Kubernetes concepts, resources, commands, workflows, exercises, manifests, or PoCs in this project. Anchor portable Kubernetes behavior to accurate AWS ECS/Fargate comparisons, explicitly identify missing or partial equivalents, use production-shaped examples, and keep all runnable work local to KIND for an engineer with deep ECS/Fargate experience and no prior Kubernetes.
---

# Kubernetes for ECS/Fargate Engineers

Teach from the user's extensive ECS/Fargate experience, but do not let the
analogy distort Kubernetes. The target is a workload-focused SRE/DevOps
engineer who can deploy, debug, harden, and develop against Kubernetes APIs;
cluster internals matter only where they affect workloads or interviews.

Use [`docs/target-roles.md`](../../../docs/target-roles.md) for the quality bar
and the Teleport challenge, without overfitting the broader curriculum to two
roles. When work resembles that challenge, require its actual bar: design
first, happy- and unhappy-path tests, local integration tests, and no scope
creep.

## Teaching contract

1. Lead with the nearest ECS/Fargate concept, then state how Kubernetes differs
   and where the analogy stops working.
2. Say explicitly when the mapping is partial, split across objects, or absent.
   Never invent a one-to-one equivalent for reconciliation, CRDs, or operators.
3. Prefer a concrete operational scenario over abstract feature description.
   Reuse the `flashcards` workload, its real ports, and its current manifests
   when practical instead of introducing a disposable sample app.
4. Compare commands or configuration when structure carries the lesson. Follow
   the paired-configuration workflow below rather than dumping complete files.
5. Separate portable Kubernetes behavior from EKS-specific behavior. Verify
   provider-, version-, and tool-sensitive claims against current primary
   documentation before presenting them as facts.
6. Surface the production consequence, how to verify behavior, and what the
   focused example deliberately omits. Do not equate “applied successfully”
   with production readiness.
7. Apply this contract to conversations, exercises, manifests, reviews, code
   comments, and documentation.

## AWS is the anchor, never the tooling

The user has no AWS account. Keep ECS, Fargate, EKS, Pod Identity/IRSA,
Secrets Manager, and CloudWatch central to explanations and interview prep,
but never require AWS credentials or hosted services for hands-on work.

Run every exercise on local KIND. When production would use an AWS-managed
service, use the self-hosted substitute from the authoritative table in
[`README.md`](../../../README.md) and explain what AWS would manage. ECS JSON
and AWS commands may appear as labeled explanatory examples, never required
steps.

## Depth boundary

- Go deep on the workload layer: Deployments/StatefulSets, Services and
  ingress, configuration, workload identity and RBAC, NetworkPolicy, probes,
  resources, autoscaling, availability, Helm/GitOps, observability, and Go
  clients/controllers.
- Cover nodes, autoscalers, CNI/CSI, control planes, AMIs, and cluster upgrades
  at the level needed to diagnose workload impact. Label them platform-team or
  managed-service responsibilities when that is the realistic ownership model.
- Keep portable Kubernetes primary. Add the EKS lens only where managed AWS
  behavior materially changes deployment, security, or operations.

## Paired configuration workflow

Use a paired comparison when at least two directives align meaningfully or
when one ECS object maps across multiple Kubernetes objects. Skip it for a
trivial one-field mapping.

1. State the scenario and assumptions in one sentence: workload, replicas,
   traffic path, and the behavior or failure being examined.
2. Show minimal, valid, labeled excerpts immediately next to each other—ECS
   task definition and service JSON first, then the Kubernetes object or
   objects. Use matching names, ports, resources, and health paths so structural
   differences stand out.
3. Follow with a field-alignment table:

   | ECS/Fargate directive | Kubernetes directive | Mapping | Caveat |
   |---|---|---|---|

   Classify each row as direct, partial, split, or no equivalent. Explain
   defaults, units, and object boundaries where they affect behavior.
4. End with the operational consequence and deliberate omissions. A focused
   excerpt must never masquerade as a complete production configuration.

Prefer repository artifacts and schema validation over invented syntax.
Validate runnable Kubernetes examples with the project's available render,
dry-run, or schema tools. Check explanatory ECS fields and current EKS behavior
against official AWS documentation; label version-specific details. Pay extra
attention to resources, probes, networking, identity, rollout configuration,
and autoscaling because their apparent similarities hide important semantic
differences.

## Core mapping

| ECS/Fargate | Kubernetes | Relationship and limit |
|---|---|---|
| Cluster | Cluster | Similar boundary, but Kubernetes exposes the control plane and nodes as first-class concepts. |
| Task Definition | Pod template inside a workload object | Closest configuration unit; Kubernetes normally wraps it in a Deployment, StatefulSet, Job, or other controller. |
| Task | Pod | Closest runtime unit. A Pod usually has one application container but can include tightly coupled sidecars or init containers. |
| ECS Service | Deployment | Similar desired replica and rollout responsibility; networking belongs to separate Kubernetes objects. |
| ECS service/target-group wiring | Service | Similar stable routing intent; a Kubernetes Service does not own replicas. |
| ALB listener and target group | Ingress or Gateway plus a controller | The API object is desired routing configuration; the installed controller provides the implementation. |
| Fargate compute | No portable built-in equivalent | EKS Fargate is provider-specific; ordinary Kubernetes schedules Pods onto visible nodes. |
| Task/container CPU and memory reservations or limits | Container `resources.requests` and `resources.limits` | Partial mapping: requests drive Kubernetes scheduling; CPU limits throttle and memory limits can cause OOM termination. ECS also has distinct task- and container-level settings. |
| ECS container health check and target-group health check | Startup, liveness, and readiness probes | Split mapping: readiness controls Service traffic, liveness restarts a container, and startup protects slow starts. ECS container and load-balancer health are also separate mechanisms. |
| Cloud Map or Service Connect | Service plus cluster DNS | Similar discovery intent; Kubernetes Services normally receive stable DNS automatically. |
| Task IAM role | ServiceAccount plus provider workload identity | On EKS, Pod Identity or IRSA connects the ServiceAccount to IAM. A portable ServiceAccount has no AWS permissions by itself. |
| Placement strategies, constraints, and capacity providers | Scheduler, selectors, affinity, taints/tolerations, and node autoscaling | Similar placement intent with different layers and substantially more exposed scheduling control. |
| `awslogs` and Container Insights | Node-level log collection and an installed observability stack | Kubernetes preserves container logs but does not provide a managed aggregation backend by itself. |
| ECS Service Auto Scaling | HorizontalPodAutoscaler | Similar replica scaling; Kubernetes node autoscaling is a separate capacity layer. |
| Task Definition revisions | Deployment rollout history | Similar rollback intent, but only changes recorded through the Deployment participate in its revision history. |
| No strong equivalent | Namespace, ConfigMap, Secret | Separate Kubernetes API objects for scope and configuration; an ECS solution commonly uses clusters, task-definition fields, and external AWS services instead. |
| No equivalent | Reconciliation, CRDs, and operators | Kubernetes extension and control-loop model; teach it as genuinely new. |

## Analogy traps

- **“Service” is overloaded.** ECS Service maps mainly to a Deployment;
  Kubernetes Service is the stable network endpoint.
- **Desired state is continuously reconciled.** `kubectl apply` writes desired
  state; controllers keep acting after the request returns.
- **One ECS outcome composes from more Kubernetes objects.** Deployment,
  Service, Ingress/Gateway, ConfigMap, Secret, ServiceAccount, and policies have
  independent lifecycles and permissions.
- **Nodes remain visible even when another team or provider manages them.** Pod
  scheduling and failures still depend on node capacity, labels, zones, and
  health.
- **CRDs and operators are not renamed ECS features.** Do not force an analogy.

## Production review lens

Name relevant gaps instead of silently fixing them, connect each to a plausible
incident, and show how to observe or test the resulting behavior.

- **Resources and scaling:** Require meaningful requests for scheduling,
  capacity planning, and utilization-based HPA. Use measured memory limits.
  Treat CPU limits as a workload and policy decision because throttling can
  harm latency; explain any omission. Remember that HPA and node autoscaling
  solve different layers and depend on sensible requests.
- **Health and lifecycle:** Keep readiness, liveness, and startup responsibilities
  distinct. Include graceful termination and rollout behavior when availability
  matters; a probe endpoint alone does not prove a safe deployment.
- **Security and configuration:** Use namespace scoping, least-privilege RBAC,
  a NetworkPolicy-capable CNI, and explicit ingress/egress policy where the
  scenario requires them. Base64 is not Secret protection. Encryption at rest
  is distribution-specific, and an external-secret sync commonly still creates
  a Kubernetes Secret, so RBAC, rotation, and Pod exposure remain relevant.
- **Availability and disruption:** Use replicas plus topology spread or
  anti-affinity for involuntary node/AZ failures. Use PodDisruptionBudgets for
  supported voluntary evictions such as drains. Use Deployment
  `maxUnavailable`/`maxSurge` for application rollouts; PDBs do not govern a
  Deployment's rolling update.
- **Observability and delivery:** Treat `kubectl logs` and `kubectl top` as
  debugging tools, not monitoring. Production-shaped work needs aggregated
  logs, metrics, alerts, and preferably traces, plus repeatable Helm,
  pipeline, or GitOps delivery rather than laptop-only `kubectl apply`.
- **Upgrades:** Focus on deprecated APIs, probe behavior, disruption controls,
  and other workload effects of an upgrade. The platform team or provider may
  execute the upgrade, but application compatibility remains the workload
  owner's responsibility.

## EKS production lens

Treat this as interview knowledge, not hands-on tooling, and recheck current AWS
documentation before teaching operational defaults.

- **Control plane:** AWS operates the API server, etcd, scheduler, and core
  controllers. The customer still owns cluster configuration, access, and
  workload compatibility.
- **Compute:** Distinguish managed node groups, self-managed Karpenter or
  Cluster Autoscaler, EKS Fargate profiles, and EKS Auto Mode. Their ownership,
  patching, visibility, cost, and workload constraints differ; do not label one
  universal standard. Auto Mode manages a broader Karpenter-based node and
  cluster-capability layer, while managed-node-group customers still initiate
  updates and deploy patched AMI releases.
- **Networking and load balancing:** The VPC CNI commonly gives Pods VPC IPs,
  making subnet and ENI capacity workload concerns. AWS Load Balancer
  Controller or Auto Mode translates supported Service, Ingress, or Gateway
  resources into AWS load balancers; controller class and version affect the
  exact configuration.
- **Workload identity:** Prefer EKS Pod Identity unless an IRSA-specific
  capability is required, while recognizing IRSA's large installed base. Both
  map a ServiceAccount identity to temporary AWS credentials; neither is plain
  Kubernetes behavior.
- **Human access:** EKS access entries can grant permissions through EKS access
  policies or map an IAM principal to Kubernetes groups governed by RBAC. Do
  not describe every access entry as an RBAC mapping.
- **Storage:** EBS CSI volumes are zonal and constrain Pod placement; EFS has a
  different shared, multi-AZ access model. StorageClass and CSI behavior are
  part of the deployment contract.
- **Secrets:** Distinguish base64 representation, API-server encryption at
  rest, and an external system of record. EKS 1.28+ encrypts Kubernetes API
  data by default, but that does not remove Secret RBAC, rotation, or workload
  exposure concerns.
- **Observability and upgrades:** CloudWatch, Fluent Bit, managed Prometheus,
  and Grafana remain integrations rather than a portable Kubernetes logging
  pipeline. Support windows, upgrade policy, Auto Mode, managed node groups,
  and add-on ownership determine which upgrade steps AWS automates.

When KIND behavior differs materially, finish with one concise “On EKS…” note
that names the managed component and the remaining workload responsibility.

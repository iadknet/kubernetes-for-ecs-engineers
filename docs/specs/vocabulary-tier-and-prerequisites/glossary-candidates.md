# Glossary candidates — the mechanical shortlist and what happened to each

The Phase 1 shortlist was produced mechanically, then curated by hand. This
file records the curation so the judgment is reviewable rather than implicit in
the deck.

## Method

A term is a **candidate** if it is emphasised (`**bold**`) anywhere in the
library *and* occurs as a whole word on three or more cards, counting `q`, `a`,
and `ecs`. Matching is case sensitive and treats `-` as a word character, so
`proxy` does not match inside `kube-proxy` and `Service` does not match inside
`ServiceAccount`.

That rule produced **67 candidates** over the 255 cards as they stood before
this change. The spec estimated 91; the difference is the matching rule, not
the corpus — a case-insensitive substring scan produces 109 candidates, most of
them junk (`E`, `IN`, `RED` matching inside unrelated words). The stricter rule
is the one the Phase 3 lint uses, so the shortlist and the enforcement now
agree.

The rule is a shortlist, not an oracle. It has two known blind spots, both
handled by curation below:

- It misses terms that are never emphasised anywhere. `Pod` occurs on 91 cards
  and `kubectl` on 45, but neither was ever bolded.
- It catches ordinary English (`not`, `one`, `must`, `set`) and prose fragments
  (`No ECS equivalent`).

## Accepted — a glossary card teaches it

`Kubernetes`, `Pod`, `API server` (aliases `kube-apiserver`, `apiserver`),
`RBAC`, `Deployment`, `Service`, `controller`, `ServiceAccount`, `Role`,
`etcd`, `CustomResourceDefinition` (alias `CRD`), `KIND`, `kubelet`,
`ClusterRole`, `desired state`, `actual state`, `label`, `ClusterRoleBinding`,
`DaemonSet`, `RoleBinding`, `replica`, `kube-controller-manager` (from
`Manager`), `PersistentVolumeClaim` (alias `PVC`), `ClusterIP`,
`PodDisruptionBudget`, `mTLS`, `metrics-server`.

## Rejected

### Ordinary English, not vocabulary

`not` (58), `one` (54), `must` (19), `set` (14), `read` (9), `current` (12),
`groups` (12), `store` (6), `previous` (4), `restarts` (8), `Ready` (5),
`Pending` (6), `watch` (7), `daemon` (3), `username` (4), `Resource` (4),
`O` (3).

Each is bolded somewhere for emphasis rather than as a term. `watch` and
`Ready` are genuine Kubernetes vocabulary in other contexts, but every
occurrence here is the English word; if that changes, they become candidates
again.

### Prose fragments the rule cannot distinguish from terms

`No ECS equivalent` (8). This is the deck's standing phrase for "this concept
has no ECS analogue", not a term to learn.

### Already known to the reader — ECS and AWS vocabulary

`Task` (17), `Task Role` (6), `Fargate` (5), `ALB` (7), `NLB` (4), `IRSA` (7),
`Pod Identity` (8), `EKS Pod Identity` (5), `single-AZ` (3),
`ReadWriteOnce` (4).

The premise of this program is that the reader already has this vocabulary —
it is the anchor, not the material. Teaching it back would invert the whole
approach. (`ReadWriteOnce` is Kubernetes, not AWS, but it is taught in place in
M3 where the access modes are compared against each other; a term card would
teach one third of a set that only makes sense whole.)

### Taught in place, in the module that owns them

`Prometheus` (9), `gRPC` (5), `CEL` (3), `GVR` (3), `BestEffort` (4),
`RED` (4), `Secrets Store CSI driver` (3), `HPA` (8, but see below).

These are tool names or module-local vocabulary rather than cross-cutting
terms. They are drained from the Phase 3 allowlist when their module is
reached, and any that turn out to be load-bearing across modules get promoted
to a term card then.

### Case collision

`Kind` (19) vs `KIND` (11). `KIND` the tool has a card. `kind` the object field
does not: with case-sensitive matching the two would be separate terms, and a
card teaching "kind" the field in isolation teaches nothing that
`m0-object-shape` does not already teach in context.

## Added by curation, below the mechanical threshold

The mechanical rule only shortlists emphasised terms, so the most-used words in
the library were invisible to it. These were added by hand, with their whole-word
card counts:

`Pod` (91), `Deployment` (48), `Service` (47), `node` (46), `kubectl` (45),
`Namespace` (23), `Secret` (15), `manifest` (14), `probe` (14), `spec` (13),
`rollout` (13), `informer` (12), `status` (12), `annotation` (12), `CNI` (11),
`endpoint` (10), `Event` (10), `ConfigMap` (10), `volume` (10), `operator` (9),
`Helm` (9), `Job` (9), `kube-scheduler` (8), `selector` (8),
`readiness probe` (7), `NetworkPolicy` (6), `Ingress` (6), `kube-proxy` (6),
`reconciliation` (6), `chart` (6), `rolling update` (6), `drain` (5),
`client-go` (5), `kubeconfig` (5), `sidecar` (5), `webhook` (5),
`ReplicaSet` (4), `StatefulSet` (4), `custom resource` (4), `CoreDNS` (3),
`eviction` (3), `SIGTERM` (3), `taint` (3), `affinity` (3), `cluster`,
`container`, `CRI`.

The last three are below the threshold as words but sit at the root of the
dependency graph: nearly every other term card requires one of them, so
teaching them explicitly is what makes the rest of the chain honest.

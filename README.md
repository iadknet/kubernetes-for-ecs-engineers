# Kubernetes Training Program

[![License: MIT](https://img.shields.io/github/license/iadknet/kubernetes-for-ecs-engineers)](LICENSE)

This is a mostly vibe coded experiment to see if I can build an app to help
me translate my experience from ECS/Fargate into Kubernetes.

The flashcards have been useful in helping me get more concrete familiarity
with Kubernetes vocabulary and I have found the chatbot extremely useful for
diving into more detail when I need more context and explanation.

The LLM is pretty terrible at wording the questions and I have been working on
creating a set of skills to try and guide it toward writing better teaching materials
(with limited success). There is still no replacement for a good human instructional designer.

The most useful artifact in the project for me has been the skill that guides the LLM
in grounding any Kubernetes explanation in ECS/Fargate experience.


<p align="center">
  <img src="docs/images/drill.gif" width="720"
       alt="The flashcard drill in action: M1 core-workloads cards — Pod, Deployment, and Service — each reveal their answer and a 'Coming from ECS' callout (Pod maps to a Task, Deployment/ReplicaSet to Service + Task Definition revisions, Service types to target-group wiring), then grade and advance.">
  <br>
  <sub><b>Drill</b> — reveal a card's answer and its "Coming from ECS" callout, then grade it <em>Again / Hard / Good / Easy</em>.</sub>
</p>

<p align="center">
  <img src="docs/images/chat.gif" width="720"
       alt="The card's chat panel: asking 'Can you give me examples of the config in Kubernetes and the same config in ECS and explain how they align?' and Claude answering with an ECS task-definition JSON example, the equivalent ConfigMap YAML, and a table aligning the two — env vars in the task def vs data: keys, new task-def revision vs editing the ConfigMap.">
  <br>
  <sub><b>Ask about a card</b> — a built-in tutor (<code>make run-chat</code>) answers in ECS terms, explaining around the card rather than through it.</sub>
</p>

## Start here

Run the study app:

```bash
cd flashcards
make run       # http://localhost:8080
# Or: make run-chat to add the local tutor panel.
```

Then begin with **[M0: local cluster setup](modules/00-setup/README.md)**. The
[flashcard guide](flashcards/README.md) covers drilling, checkpoints, chat, and
configuration.

## How the program works

Each module follows one loop: learn the Kubernetes concept through its nearest
ECS/Fargate analogue, exercise it on local KIND, review the result against a
production bar, then advance. Modules are written just in time so later work
builds on what happened in practice.

From M1 onward, the flashcard service is also the workload being deployed and
hardened. The program eventually turns it into a Go client of the Kubernetes
API.

## Local substitutions

Everything hands-on runs on KIND. No AWS account or cloud credentials are
required. AWS remains the teaching and interview-preparation lens.

| Production on AWS | Local equivalent | What the comparison teaches |
|---|---|---|
| Secrets Manager / SSM Parameter Store | HashiCorp Vault + External Secrets Operator | ECS `secrets` / `valueFrom` |
| IRSA / EKS Pod Identity | Vault Kubernetes auth | Task Role → workload identity |
| ALB + AWS Load Balancer Controller | ingress-nginx | Portable Ingress vs. its controller |
| CloudWatch Container Insights / `awslogs` | Prometheus + Grafana + Loki | Kubernetes has no built-in logging pipeline |
| ECR | `kind load docker-image` | Registry auth and image pull secrets |
| EKS managed control plane | KIND: 1 control-plane + 2 workers | What AWS manages and what remains visible |

## The roadmap

The marker changes only when a module meets its **Done when** criteria: ✅ done ·
🚧 in progress · ⏳ not started.

| # | Focus | ECS anchor | Target |
|---|---|---|---|
| 🚧 M0 | Local production-shaped cluster | ECS cluster, with visible control plane and nodes | All levels |
| ⏳ M1 | Pods, Deployments, Services | Task Definition → Pod spec; ECS Service → Deployment; target-group wiring → Service | L1–L2 |
| ⏳ M2 | Config, Secrets, Namespaces, RBAC | Task environment and secret references → ConfigMap/Secret; Task Role → ServiceAccount | L3, L5 |
| ⏳ M3 | Resources, probes, scaling, availability, Helm, Ingress | Task resources, health checks, service scaling, ALB | L3 |
| ⏳ M4 | Prometheus, Grafana, Loki | CloudWatch Container Insights / `awslogs` | L3 + SRE role |
| ⏳ M5 | Go clients, informers, watches | One-shot ECS API calls → a long-running client | L1–L4 |
| ⏳ M6 | CRDs and controllers | No ECS equivalent | L5 |
| ⏳ M7 | Cluster authentication and short-lived credentials | IAM → authentication plus RBAC; Kubernetes has no User object | Interview depth |
| ⏳ CAP | Teleport SRE take-home rehearsal | The complete system | L1–L5 |

See the **[curriculum](docs/curriculum.md)** for objectives, exercises, and
completion criteria.

## Project map

| Path | Purpose |
|---|---|
| [modules/](modules/) | Hands-on module guides and artifacts |
| [flashcards/](flashcards/) | Study app, decks, and example Go workload |
| [docs/curriculum.md](docs/curriculum.md) | Full module curriculum |
| [docs/target-roles.md](docs/target-roles.md) | Roles and take-home challenges that set the bar |
| [docs/specs/](docs/specs/) | Technical specs for non-trivial work |
| [AGENTS.md](AGENTS.md) | Project workflow and conventions |

## Working agreement

Exercises are judged by production behavior, not merely by whether they run.
The program emphasizes Kubernetes development in Go from M5 onward. Its
capstone is rehearsal; the real interview submission remains my own work.

## Security and license

Report vulnerabilities privately through [SECURITY.md](SECURITY.md). Released
under the [MIT License](LICENSE). © 2026 Isaac Stefanek.

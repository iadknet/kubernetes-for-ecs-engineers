# M0 — Local production-shaped cluster

**Goal:** Install the toolchain and stand up a **multi-node** Kubernetes
cluster on your Mac, then understand what you're looking at.

**Time:** ~30–45 min, most of it downloads.

**Your machine:** Apple Silicon (arm64), macOS 26, Homebrew not yet installed.
The steps below account for that.

---

## The mental model (read first)

Coming from ECS/Fargate, here's the reframing for what you're about to build:

- A **KIND cluster** is like standing up an ECS cluster — except KIND runs the
  whole Kubernetes cluster *inside Docker containers on your laptop* ("KIND" =
  Kubernetes IN Docker). Each "node" is a container pretending to be a machine.
- Unlike ECS, the **control plane** (API server, scheduler, etcd) and the
  **worker nodes** are first-class things you can see and poke at. `kubectl get
  nodes` has no real ECS equivalent — ECS hides all of that.
- **`kubectl`** is your `aws ecs` CLI equivalent. A **kubeconfig context** is
  like an AWS profile + region: it points `kubectl` at one specific cluster and
  namespace.
- We build a **3-node** cluster (1 control-plane + 2 workers) on purpose. A
  single-node cluster hides scheduling, disruption, and node-failure behavior —
  the exact things that matter in production. This is the project's
  production-realism rule applied from step one.

---

## Step 1 — Install Homebrew

Homebrew is the standard macOS package manager; we'll use it for `kind` and
`kubectl`. In your terminal:

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

> Tip: in this Claude Code session you can prefix a command with `! ` to run it
> here so its output lands in the conversation — handy if you want me to see any
> errors.

After it finishes, follow its printed instructions to add `brew` to your PATH
(on Apple Silicon it's usually these two lines), then verify:

```bash
echo 'eval "$(/opt/homebrew/bin/brew shellenv)"' >> ~/.zprofile
eval "$(/opt/homebrew/bin/brew shellenv)"
brew --version
```

## Step 2 — Install Docker Desktop

KIND needs a container runtime. We're using Docker Desktop (closest to your ECS
container muscle memory, free for personal use).

```bash
brew install --cask docker
```

Then **launch Docker Desktop from Applications once** and let it finish
starting (whale icon in the menu bar goes steady). Accept the default settings.
Verify the daemon is up:

```bash
docker info
```

You should get a block of server info, not a "cannot connect" error.

## Step 3 — Install kind and kubectl

```bash
brew install kind kubectl
kind version
kubectl version --client
```

---

## Step 4 — Create the cluster (the exercise)

A cluster config file is committed alongside this README:
[`kind-cluster.yaml`](kind-cluster.yaml). It declares one control-plane node and
two workers. Create the cluster from it:

```bash
kind create cluster --name k8s-training --config modules/00-setup/kind-cluster.yaml
```

(Run from the repo root.)

KIND automatically adds a kubeconfig **context** named `kind-k8s-training` and
switches to it. Confirm which cluster `kubectl` is pointed at:

```bash
kubectl config current-context
```

---

## Step 5 — Look around

```bash
# The nodes — no ECS equivalent for seeing your control plane like this
kubectl get nodes -o wide

# The control-plane components + core add-ons, running as pods
kubectl get pods -n kube-system

# All the contexts kubectl knows about (like your AWS profiles)
kubectl config get-contexts

# Cluster-level info
kubectl cluster-info
```

Things to notice, ECS-side-by-side:

- `kubectl get nodes` shows 3 nodes: one `control-plane`, two `<none>` (worker)
  roles. On ECS you'd never see the control plane — AWS runs it invisibly.
- The `kube-system` namespace runs the cluster's own machinery (API server,
  scheduler, CoreDNS, CNI) *as pods*. Kubernetes runs itself on itself.
- CoreDNS is the in-cluster DNS — this is what makes Service discovery work
  later (closer to ECS Service Connect than to Cloud Map's opt-in model).

---

## Done when

`kubectl get nodes` shows **one control-plane and two worker nodes, all
`Ready`**, from the committed config file. Something like:

```
NAME                          STATUS   ROLES           AGE   VERSION
k8s-training-control-plane    Ready    control-plane   2m    v1.xx.x
k8s-training-worker           Ready    <none>          90s   v1.xx.x
k8s-training-worker2          Ready    <none>          90s   v1.xx.x
```

## When you're done

Paste the output of `kubectl get nodes -o wide` back here and I'll review it,
answer any "wait, why does X work that way vs ECS?" questions, and we'll start
**M1 — Core workloads**.

---

## Cleanup / resume reference

You do **not** need to delete the cluster between sessions — leave it running.
For reference only:

```bash
kind get clusters                       # list clusters
kind delete cluster --name k8s-training # tear it down completely
kind create cluster --name k8s-training --config modules/00-setup/kind-cluster.yaml  # recreate
```

If Docker Desktop isn't running, the cluster's containers are stopped; starting
Docker Desktop brings them back.

## Troubleshooting

- **`docker info` fails / "Cannot connect to the Docker daemon":** Docker
  Desktop isn't running. Open it from Applications and wait for the whale icon.
- **`kind create cluster` hangs or fails pulling images:** usually Docker
  Desktop is still starting, or low on resources. Give Docker Desktop at least
  4 GB memory (Settings → Resources).
- **`kubectl` points at the wrong cluster later:**
  `kubectl config use-context kind-k8s-training`.

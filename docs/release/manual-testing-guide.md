# Manual Testing Guide — PD to Hyperdisk Conversion

How to stand up a cluster from scratch, deploy a locally built driver onto it, and
exercise the disk type conversion feature by hand.

Everything here has been run end to end. Where a step exists only because something
silently fails without it, that is called out.

---

## Table of contents

1. [Prerequisites](#1-prerequisites)
2. [Choosing a Kubernetes version](#2-choosing-a-kubernetes-version)
3. [Creating the VMs](#3-creating-the-vms)
4. [Bootstrapping the nodes](#4-bootstrapping-the-nodes)
5. [Creating the cluster](#5-creating-the-cluster)
6. [Enabling VolumeAttributesClass](#6-enabling-volumeattributesclass)
7. [Building and loading the driver image](#7-building-and-loading-the-driver-image)
8. [Deploying the driver](#8-deploying-the-driver)
9. [Test fixtures](#9-test-fixtures)
10. [Test procedures](#10-test-procedures)
11. [Reading the results](#11-reading-the-results)
12. [Troubleshooting](#12-troubleshooting)
13. [Cleanup](#13-cleanup)

---

## 1. Prerequisites

On your workstation:

| Tool | Why |
|---|---|
| `gcloud` (authenticated) | creating VMs, inspecting disks |
| `docker` | building the driver image |
| `go` 1.24+ | building and running unit tests |

Check authentication before you start — an expired token fails in a confusing way
part way through:

```bash
gcloud auth login          # if needed
gcloud config get-value project
gcloud compute instances list   # should not error
```

In the GCP project you will need:

- The alpha `disks.convert` API allowlisted. Without it every conversion is refused.
- Quota for Hyperdisk in the target region.
- A VPC with internal traffic allowed between nodes and SSH reachable.

This guide uses `<your-vpc>` / subnet `<your-subnet>` in `us-central1-b`, which
already permits all internal traffic. If you use a different network, open at least:
`tcp:6443`, `tcp:10250`, `tcp:2379-2380`, `udp:8472` (Flannel VXLAN), and `tcp:22`.

---

## 2. Choosing a Kubernetes version

The VolumeAttributesClass API differs significantly between versions. Pick
deliberately — this decides two later steps.

| | **1.33** | **1.34+** |
|---|---|---|
| VAC API version | `storage.k8s.io/v1beta1` | `storage.k8s.io/v1` |
| Feature gate | **must be enabled manually** | **on by default** |
| Required csi-resizer | **v1.13.2** (watches v1beta1) | **v2.0.0** (watches v1) |
| Clearing VAC to `null` | always `Forbidden` | allowed only while `status.currentVolumeAttributesClassName` is nil |

Use **1.33** to reproduce current customer conditions, **1.34** to test the
cancellation-by-null path.

---

## 3. Creating the VMs

Two nodes are enough: an `e2-standard-2` control plane and one `c3-standard-4`
worker.

> **The worker must be Hyperdisk-capable — C3, C4 or N4.** E2 and N2 machine types
> cannot attach a Hyperdisk. A conversion will appear to succeed and then the pod
> will never mount. This costs hours if you get it wrong.

Spot instances are much cheaper and fine for this, as long as you accept the
occasional preemption (see [Troubleshooting](#12-troubleshooting)).

```bash
ZONE=us-central1-b
NET_ARGS="--network=<your-vpc> --subnet=<your-subnet>"
COMMON="--provisioning-model=SPOT --instance-termination-action=STOP \
  --image-family=ubuntu-2204-lts --image-project=ubuntu-os-cloud \
  --boot-disk-size=50GB --boot-disk-type=pd-balanced \
  --scopes=https://www.googleapis.com/auth/cloud-platform"

gcloud compute instances create test-cp --zone=$ZONE \
  --machine-type=e2-standard-2 $COMMON $NET_ARGS

gcloud compute instances create test-w1 --zone=$ZONE \
  --machine-type=c3-standard-4 $COMMON $NET_ARGS
```

The `cloud-platform` scope matters: it lets the driver authenticate from the VM's
metadata server, so you never need to create or download a service-account key.

---

## 4. Bootstrapping the nodes

Save this as `bootstrap.sh`. Set `K8S_MINOR` to the version you chose in step 2.

```bash
#!/bin/bash
set -euxo pipefail
K8S_MINOR=v1.34        # or v1.33

sudo swapoff -a
sudo sed -i '/ swap / s/^/#/' /etc/fstab

cat <<'M' | sudo tee /etc/modules-load.d/k8s.conf
overlay
br_netfilter
M
sudo modprobe overlay
sudo modprobe br_netfilter

cat <<'S' | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
S
sudo sysctl --system

export DEBIAN_FRONTEND=noninteractive
sudo apt-get update -q
sudo apt-get install -yq apt-transport-https ca-certificates curl gpg containerd conntrack

sudo mkdir -p /etc/containerd
containerd config default | sudo tee /etc/containerd/config.toml >/dev/null
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
sudo systemctl restart containerd
sudo systemctl enable containerd

sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://pkgs.k8s.io/core:/stable:/${K8S_MINOR}/deb/Release.key \
  | sudo gpg --dearmor --yes -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/${K8S_MINOR}/deb/ /" \
  | sudo tee /etc/apt/sources.list.d/kubernetes.list
sudo apt-get update -q
sudo apt-get install -yq kubelet kubeadm kubectl
sudo apt-mark hold kubelet kubeadm kubectl
sudo systemctl enable kubelet
kubeadm version -o short
echo "BOOTSTRAP_DONE"
```

`SystemdCgroup = true` is required — kubelet and containerd must agree on the
cgroup driver or the kubelet will not start.

Run it on both nodes in parallel:

```bash
for n in test-cp test-w1; do
( gcloud compute scp bootstrap.sh $n:~/b.sh --zone=$ZONE --quiet >/dev/null 2>&1 && \
  gcloud compute ssh $n --zone=$ZONE --quiet --command="chmod +x ~/b.sh && sudo ~/b.sh" \
    > boot-$n.log 2>&1 ) &
done; wait
grep -c BOOTSTRAP_DONE boot-*.log      # expect a non-zero count for each
```

A fresh VM sometimes refuses SSH for the first minute or so. If one node fails,
just re-run it.

---

## 5. Creating the cluster

**Initialise the control plane:**

```bash
gcloud compute ssh test-cp --zone=$ZONE --quiet --command="
  sudo kubeadm init --pod-network-cidr=10.244.0.0/16 \
    --apiserver-advertise-address=\$(hostname -I | awk '{print \$1}')" 2>&1 | tail -20
```

Keep the `kubeadm join` command it prints.

**Configure kubectl, install Flannel, and remove the control-plane taint:**

```bash
gcloud compute ssh test-cp --zone=$ZONE --quiet --command="
mkdir -p \$HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf \$HOME/.kube/config
sudo chown \$(id -u):\$(id -g) \$HOME/.kube/config
kubectl apply -f https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml
kubectl taint nodes --all node-role.kubernetes.io/control-plane-"
```

> Removing the taint matters on a two-node cluster. The driver's DaemonSet must
> cover the control plane, and leaving the taint on has previously caused a
> hostPort scheduling deadlock during rollout.

**Join the worker** using the command from `kubeadm init`:

```bash
gcloud compute ssh test-w1 --zone=$ZONE --quiet --command="sudo kubeadm join 10.170.0.30:6443 \
  --token <token> --discovery-token-ca-cert-hash sha256:<hash>"
```

**Verify:**

```bash
gcloud compute ssh test-cp --zone=$ZONE --quiet --command="kubectl get nodes -o wide"
```

Both nodes should be `Ready` within a minute or two.

---

## 6. Enabling VolumeAttributesClass

**On 1.34+ — skip this section.** The gate is on by default. Confirm with:

```bash
kubectl api-resources | grep -i volumeattributes
# volumeattributesclasses   vac   storage.k8s.io/v1   false   VolumeAttributesClass
```

**On 1.33 — required.** Without it the API is not served at all:

```bash
gcloud compute ssh test-cp --zone=$ZONE --quiet --command="
sudo python3 - <<'PYEOF'
p='/etc/kubernetes/manifests/kube-apiserver.yaml'
s=open(p).read()
if 'VolumeAttributesClass' not in s:
    s=s.replace('    - kube-apiserver\n',
        '    - kube-apiserver\n'
        '    - --feature-gates=VolumeAttributesClass=true\n'
        '    - --runtime-config=storage.k8s.io/v1beta1=true\n',1)
    open(p,'w').write(s); print('apiserver patched')

p2='/etc/kubernetes/manifests/kube-controller-manager.yaml'
s2=open(p2).read()
if 'VolumeAttributesClass' not in s2:
    s2=s2.replace('    - kube-controller-manager\n',
        '    - kube-controller-manager\n'
        '    - --feature-gates=VolumeAttributesClass=true\n',1)
    open(p2,'w').write(s2); print('controller-manager patched')
PYEOF
sleep 60
kubectl api-resources | grep -i volumeattributes"
```

The static pods restart themselves. Expect the API to be briefly unavailable.

---

## 7. Building and loading the driver image

The build **requires** `STAGINGVERSION`; without it the Makefile aborts with
`Must set environment variable GCE_PD_CSI_STAGING_VERSION`.

Use a distinct tag per build. The version string is visible at runtime, which is
how you confirm the cluster is running the code you think it is.

```bash
cd <repo root>
docker build -f Dockerfile --build-arg STAGINGVERSION=v15-lro -t custom-pdcsi:v15 .
docker save custom-pdcsi:v15 | gzip -1 > /tmp/pdcsi-v15.tar.gz     # ~127 MB
```

Import into containerd on **every** node — the image is local, so there is no
registry to pull from:

```bash
for n in test-cp test-w1; do
( gcloud compute scp /tmp/pdcsi-v15.tar.gz $n:~/p.tar.gz --zone=$ZONE --quiet >/dev/null 2>&1 && \
  gcloud compute ssh $n --zone=$ZONE --quiet --command="
    gunzip -c ~/p.tar.gz | sudo ctr -n k8s.io images import - >/dev/null 2>&1
    sudo ctr -n k8s.io images ls | grep -c custom-pdcsi:v15
    rm ~/p.tar.gz" > imp-$n.log 2>&1 ) &
done; wait
cat imp-*.log      # each should print 1
```

The `-n k8s.io` namespace is required. An image imported into the default
containerd namespace is invisible to Kubernetes.

---

## 8. Deploying the driver

Use the **`noauth` overlay**, which drops the `cloud-sa` secret and lets the driver
use the VM's metadata credentials. No service-account key is created, so there is
nothing sensitive to clean up afterwards.

**Prepare the manifests locally:**

```bash
cp -r deploy /tmp/deploy

# point at your image
sed -i 's|newName: registry.k8s.io/cloud-provider-gcp/gcp-compute-persistent-disk-csi-driver|newName: docker.io/library/custom-pdcsi|; s|newTag: "v1.26.0"|newTag: "v15"|' \
  /tmp/deploy/kubernetes/images/stable-master/image.yaml

# turn the feature on
sed -i 's|            - --enable-multitenancy=false|            - --enable-multitenancy=false\n            - --enable-pd-conversion=true|' \
  /tmp/deploy/kubernetes/base/controller/controller.yaml
```

**On 1.33 only**, pin csi-resizer down to a version that watches `v1beta1`:

```bash
sed -i 's|newTag: "v2.0.0"|newTag: "v1.13.2"|' \
  /tmp/deploy/kubernetes/images/stable-master/image.yaml
```

**Render and apply on the control plane:**

```bash
tar czf /tmp/deploy.tar.gz -C /tmp deploy
gcloud compute scp /tmp/deploy.tar.gz test-cp:~/d.tar.gz --zone=$ZONE --quiet

gcloud compute ssh test-cp --zone=$ZONE --quiet --command="
rm -rf ~/deploy && tar xzf ~/d.tar.gz
kubectl create namespace gce-pd-csi-driver
kubectl kustomize ~/deploy/kubernetes/overlays/noauth > ~/pdcsi.yaml
sed -i 's|image: docker.io/library/custom-pdcsi:v15|image: docker.io/library/custom-pdcsi:v15\n        imagePullPolicy: IfNotPresent|' ~/pdcsi.yaml
kubectl apply -f ~/pdcsi.yaml"
```

`imagePullPolicy: IfNotPresent` is essential — the default would try to pull from
Docker Hub and fail.

### The sidecar feature gate — easy to miss

**On 1.33**, pinning csi-resizer to v1.13.2 is *not sufficient*. It also needs the
feature gate, or it runs only the resize controller and **silently ignores every
VAC change**. The only symptom is this in its log:

```
"Started PVC processing for resize controller" key="default/my-pvc"
"No need to resize PVC" PVC="default/my-pvc"
```

Container indices are `0 gce-pd-driver`, `1 taint-controller`, `2 csi-provisioner`,
`3 csi-attacher`, `4 csi-resizer`, `5 csi-snapshotter`. Verify before patching:

```bash
kubectl get deploy csi-gce-pd-controller -n gce-pd-csi-driver \
  -o jsonpath='{range .spec.template.spec.containers[*]}{.name}{"\n"}{end}' | nl -v0

kubectl patch deploy csi-gce-pd-controller -n gce-pd-csi-driver --type=json -p='[
 {"op":"replace","path":"/spec/template/spec/containers/2/args/2",
  "value":"--feature-gates=Topology=true,VolumeAttributesClass=true"},
 {"op":"add","path":"/spec/template/spec/containers/4/args/-",
  "value":"--feature-gates=VolumeAttributesClass=true"}
]'
kubectl rollout status deploy/csi-gce-pd-controller -n gce-pd-csi-driver
```

**Confirm the running version:**

```bash
CPOD=$(kubectl get pods -n gce-pd-csi-driver -o name | grep controller | head -1)
kubectl logs -n gce-pd-csi-driver $CPOD -c gce-pd-driver --tail=100 | grep vendor_version
# ... vendor_version:"v15-lro"
```

> Always target the controller pod by name. `kubectl logs deploy/csi-gce-pd-controller`
> can select a **node** DaemonSet pod instead and show you the wrong logs entirely.

---

## 9. Test fixtures

Apply once. Adjust `apiVersion` to `storage.k8s.io/v1` on 1.34+.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: sc-pd-balanced
provisioner: pd.csi.storage.gke.io
parameters:
  type: pd-balanced
volumeBindingMode: Immediate
allowVolumeExpansion: true
---
apiVersion: storage.k8s.io/v1beta1     # v1 on 1.34+
kind: VolumeAttributesClass
metadata:
  name: vac-hd-balanced
driverName: pd.csi.storage.gke.io
parameters:
  type: hyperdisk-balanced
---
apiVersion: storage.k8s.io/v1beta1
kind: VolumeAttributesClass
metadata:
  name: vac-pd-balanced-identity        # cancels a queued conversion
driverName: pd.csi.storage.gke.io
parameters:
  type: pd-balanced
---
apiVersion: storage.k8s.io/v1beta1
kind: VolumeAttributesClass
metadata:
  name: vac-hd-throughput-with-iops     # terminal: hd-throughput takes no IOPS
driverName: pd.csi.storage.gke.io
parameters:
  type: hyperdisk-throughput
  iops: "3000"
---
apiVersion: storage.k8s.io/v1beta1
kind: VolumeAttributesClass
metadata:
  name: vac-huge-iops                   # terminal: above the ceiling
driverName: pd.csi.storage.gke.io
parameters:
  type: hyperdisk-balanced
  iops: "900000"
---
apiVersion: storage.k8s.io/v1beta1
kind: VolumeAttributesClass
metadata:
  name: vac-zero-iops                   # terminal: rejected client-side
driverName: pd.csi.storage.gke.io
parameters:
  type: hyperdisk-balanced
  iops: "0"
```

> **`throughput` must carry units.** `"200Mi"` is correct. A bare `"200"` parses as
> 200 *bytes* and the API rejects it with
> `Requested provisioned throughput cannot be smaller than 140`.

---

## 10. Test procedures

Throughout:

```bash
PV=$(kubectl get pvc <name> -o jsonpath='{.spec.volumeName}')
CPOD=$(kubectl get pods -n gce-pd-csi-driver -o name | grep controller | head -1)
```

### 10.1 Happy path — conversion on a detached disk

```bash
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: demo-pvc
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: sc-pd-balanced
  resources: {requests: {storage: 200Gi}}
EOF

# wait for Bound, then trigger
kubectl patch pvc demo-pvc -p '{"spec":{"volumeAttributesClassName":"vac-hd-balanced"}}'
```

Within ~10 seconds:

```bash
kubectl get pv $PV -o jsonpath='{.metadata.annotations}' | tr ',' '\n' | grep pdcsi
```

Expect the operation selfLink and `disk-type-converted-from: pd-balanced`.

A 200 GiB conversion completes in roughly **70–90 seconds** (2 TiB took ~2m40s —
duration is not linear in size). On completion expect the operation annotation
gone, `disk-type-converted-to: hyperdisk-balanced`, and **exactly one**
`DiskTypeConversionComplete` event.

```bash
gcloud compute disks describe $PV --zone=$ZONE --format='value(type.basename())'
kubectl get events --field-selector involvedObject.name=$PV \
  -o custom-columns=TIME:.lastTimestamp,REASON:.reason
```

**Verify the data survived** by mounting it and writing to it.

### 10.2 Conversion deferred while attached (failure mode 1)

Attach a pod first, then apply the VAC. Expect:

```
annotation: Pending
ModifyVolume failed: FailedPrecondition ... while it is attached to [...]
```

Delete the pod. The conversion starts at detach:

```
detached with a conversion queued, starting it
```

### 10.3 Operation guards

While a conversion is running, each of these must be refused. The conversion is
short, so either use a large disk or use a *queued* (`Pending`) conversion, which
blocks deterministically.

| Operation | Expected |
|---|---|
| Attach | Pod `Pending`, `FailedAttachVolume` naming the operation |
| Expand | `VolumeResizeFailed ... a disk type conversion is in progress` |
| Snapshot | `Refusing to snapshot disk ...` |
| Delete | `Unavailable ... cannot delete disk ...` (**not** `InvalidArgument`) |

Snapshot tests need the snapshot controller, which is separate from the sidecar:

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.2.1/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.2.1/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.2.1/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.2.1/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v8.2.1/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml
```

### 10.4 Terminal failures (failure mode 3)

Each must give `modifyVolumeStatus: Infeasible`, a pass-through message in the PVC
conditions, and — importantly — **an empty conversion annotation**, so the volume is
not blocked.

| Case | How | Expected message |
|---|---|---|
| Zero IOPS | `vac-zero-iops` | `must be greater than zero` |
| Above ceiling | `vac-huge-iops` | `exceeds the maximum of 100000 for a 200 GiB hyperdisk-balanced disk` |
| IOPS on hd-throughput | `vac-hd-throughput-with-iops` | `cannot specify IOPS for disk type hyperdisk-throughput` |
| Unsupported target | hd-balanced → hd-throughput | `DISK_TYPE_CONVERSION_UNSUPPORTED` |
| Regional disk | see below | `not supported for regional disks` |
| Multi-writer | see below | `not supported for multi-writer disks` |

Regional and multi-writer disks cannot be provisioned through the StorageClass on a
single-zone cluster. Create them directly and bind with a static PV:

```bash
gcloud compute disks create test-regional --size=200GB --type=pd-balanced \
  --region=us-central1 --replica-zones=us-central1-b,us-central1-c

gcloud compute disks create test-mw --size=200GB --type=hyperdisk-balanced \
  --zone=$ZONE --access-mode=READ_WRITE_MANY
```

Use volume handles of the form
`projects/<project>/regions/<region>/disks/<name>` and
`projects/<project>/zones/<zone>/disks/<name>`.

### 10.5 Retriable failures (failure mode 2)

Easiest to provoke by leaving a snapshot on the disk, then converting:

```
DiskTypeConversionRetry ... failed and will be retried:
  a snapshot of the disk is still in use ... RESOURCE_IN_USE_BY_ANOTHER_RESOURCE
```

Expect the annotation to return to `Pending`, attach to stay blocked, and the
verbatim GCE error to appear in the PVC status on the next attempt.

> A conversion creates its own `temp-snapshot-<uuid>` internally. A failed
> conversion can leave one behind and block later attempts — if a disk becomes
> stubbornly unconvertible, check for it.

### 10.6 Cancellation

**Supported on all versions** — point the VAC at the disk's current type:

```bash
kubectl patch pvc demo-pvc -p '{"spec":{"volumeAttributesClassName":"vac-pd-balanced-identity"}}'
```

The queued conversion is cancelled, the `Pending` annotation is removed, and a
`DiskTypeConversionCancelled` event is emitted.

**Clearing to `null`** behaves differently by version:

```bash
kubectl patch pvc demo-pvc -p '{"spec":{"volumeAttributesClassName":null}}'
```

- **1.33:** always refused —
  `Forbidden: update from non-nil value to nil is forbidden`
- **1.34+:** allowed **only while `status.currentVolumeAttributesClassName` is nil**.
  Once any VAC has applied successfully, it is refused with
  `update to nil is forbidden when status.currentVolumeAttributesClassName is not nil`.

So `null` cancels a VAC that never took effect (e.g. an `Infeasible` one); it cannot
cancel one that already applied.

### 10.7 Durability

- Restart the controller mid-conversion (`kubectl delete pod -n gce-pd-csi-driver -l app=...`).
  The annotation must survive and the completion must still be recorded, by the
  fallback rather than the watcher.
- On spot VMs, a preemption of the control plane is a free, harsher version of this
  test.

### 10.8 Identity VAC (PRD CUJ-01)

Applying a VAC whose `type` matches the disk exactly must be treated as an
IOPS/throughput **update**, not a conversion:

```bash
# on a hyperdisk-balanced volume
kubectl patch pvc demo-pvc -p '{"spec":{"volumeAttributesClassName":"vac-hd-identity"}}'
```

Expect `status.currentVolumeAttributesClassName` to be set and **no new**
`DiskTypeConversionStart` event.

---

## 11. Reading the results

**The four places to look:**

```bash
# 1. durable conversion state
kubectl get pv $PV -o jsonpath='{.metadata.annotations}' | tr ',' '\n' | grep pdcsi

# 2. what the user sees on the claim
kubectl get pvc <name> -o jsonpath='{.status.modifyVolumeStatus}'
kubectl get pvc <name> -o jsonpath='{.status.conditions[*].message}'

# 3. events
kubectl get events --field-selector involvedObject.name=$PV \
  -o custom-columns=TIME:.lastTimestamp,REASON:.reason,MSG:.message

# 4. the truth
gcloud compute disks describe $PV --zone=$ZONE \
  --format='value(type.basename(),status,provisionedIops,provisionedThroughput,users)'
```

**Annotation states:**

| Value | Meaning |
|---|---|
| absent | no conversion in progress |
| `Pending` | queued — waiting for detach, a retry, or a free slot |
| operation selfLink | running; this is what a restarted driver polls |

**Event reasons:** `DiskTypeConversionStart`, `...Complete`, `...Retry`,
`...Cancelled`, `...Failed`.

**Driver log filters:**

```bash
kubectl logs -n gce-pd-csi-driver $CPOD -c gce-pd-driver --tail=3000 \
  | grep -iE 'convers|Refusing to|Watching'
```

---

## 12. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| VAC change does nothing; resizer logs only "No need to resize PVC" | csi-resizer missing the feature gate, or v2.0.0 on 1.33 | §8 — pin v1.13.2 **and** add the gate |
| `kubectl get vac` → API not found | gate not enabled (1.33) | §6 |
| Conversion succeeds, pod never mounts | worker is not Hyperdisk-capable | use C3/C4/N4 |
| `Requested provisioned throughput cannot be smaller than 140` | throughput given without units | use `"200Mi"` |
| Pod stuck `ContainerCreating`, image errors | image not in the `k8s.io` containerd namespace, or wrong pull policy | §7, and `imagePullPolicy: IfNotPresent` |
| `kubeadm init` preflight: `conntrack not found in system path` | `conntrack` missing on the node | install it with the other packages in §4 |
| Build fails: `Must set environment variable GCE_PD_CSI_STAGING_VERSION` | missing build arg | `--build-arg STAGINGVERSION=...` |
| `kubectl logs` shows unrelated output | `deploy/...` selected a node pod | target the controller pod by name |
| SSH dies mid-command; cluster unreachable | spot VM preempted | `gcloud compute instances start <vm>`; state on disk survives |
| Conversion retries forever on `RESOURCE_IN_USE_BY_ANOTHER_RESOURCE` with no snapshots visible | leftover conversion `temp-snapshot` | check `gcloud compute snapshots list`; may clear on its own |

**Spot preemption helper** — wrap commands so a preempted control plane is restarted
automatically:

```bash
#!/bin/bash
Z=us-central1-b
ensure_up() {
  for n in test-cp test-w1; do
    st=$(gcloud compute instances describe $n --zone=$Z --format='value(status)' 2>/dev/null)
    [ "$st" != "RUNNING" ] && gcloud compute instances start $n --zone=$Z >/dev/null 2>&1 && sleep 60
  done
}
ensure_up
for attempt in 1 2; do
  gcloud compute ssh test-cp --zone=$Z --quiet --command="$1" 2>&1 && exit 0
  ensure_up
done
exit 1
```

---

## 13. Cleanup

> Check what is yours first. Shared projects often contain other people's clusters
> and running E2E suites — deleting by wildcard is how you take down someone's work.

```bash
gcloud compute instances list
gcloud compute disks list
gcloud compute snapshots list
```

Then:

```bash
# 1. VMs (boot disks are deleted with them)
gcloud compute instances delete test-cp test-w1 --zone=$ZONE --quiet

# 2. test disks left behind by PVCs
for d in $(gcloud compute disks list --filter="zone:$ZONE AND name~^pvc-" --format='value(name)'); do
  gcloud compute disks delete $d --zone=$ZONE --quiet
done

# 3. anything created by hand
gcloud compute disks delete test-mw --zone=$ZONE --quiet
gcloud compute disks delete test-regional --region=us-central1 --quiet

# 4. snapshots
gcloud compute snapshots list
```

Verify:

```bash
gcloud compute instances list   # Listed 0 items
gcloud compute disks list       # Listed 0 items
```

Using the `noauth` overlay means **no service-account key was ever created**, so
there is no credential to revoke. If you reused an existing VPC as described, no
firewall rules were created either.

> The delete guard defers deletion while a conversion is running, so cleanup can lag
> by a conversion's duration. That is correct behaviour — re-run the delete.

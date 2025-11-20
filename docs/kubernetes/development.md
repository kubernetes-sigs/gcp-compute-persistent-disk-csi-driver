# Kubernetes Development

## Manual deployment

To build and install a development version of the driver:

```sh
GCE_PD_CSI_STAGING_IMAGE=registry.pkg.dev/path/to/driver/image   # Location to push dev image to
make push-container

# Modify controller.yaml and node.yaml in ./deploy/kubernetes/dev to use dev image
GCE_PD_DRIVER_VERSION=dev
./deploy/kubernetes/deploy-driver.sh
```

To bring down driver:

```sh
./deploy/kubernetes/delete-driver.sh
```

## Testing

### E2E tests:

The following environment variables will be required for setup and all
subsequent test runs.

```sh
export PROJECT=$USER-e2e                                             # GCP project used for testing
export GCE_PD_SA_NAME=$PROJECT-pd-sa                                 # Service account name
export IAM_NAME=$GCE_PD_SA_NAME@$PROJECT.iam.gserviceaccount.com     # Service account IAM name
export GCE_PD_SA_DIR=~/credentials                                   # Service account directory
export GOOGLE_APPLICATION_CREDENTIALS=$GCE_PD_SA_DIR/cloud-sa.json   # Service account key
```

#### One time setup

The following steps will need to be completed once per project.

##### Create service account

To create a service account with correct permissions, your test project must:

1.  Be assigned
    [Project IAM Admin](https://cloud.google.com/iam/docs/understanding-roles#resourcemanager.projectIamAdmin)
    and
    [Role Administrator](https://cloud.google.com/iam/docs/understanding-roles#iam.roleAdmin)
    IAM roles.
2.  Be part of an organization that allows creation of service account keys.
    -   For example, a Google internal developer project will not work for E2E
        testing because the org restricts SA key creation.

```sh
export ENABLE_KMS=true
export ENABLE_KMS_ADMIN=true
./deploy/setup-project.sh
```

##### Create SSH key

1.  Follow instructions to
    [generate an SSH key](https://cloud.google.com/compute/docs/connect/create-ssh-keys#create_an_ssh_key_pair)
    with the following parameters:
    -   `KEY_FILENAME: "google_compute_engine"`
    -   `USERNAME: "$USER"`
2.  Add your public key (`cat ~/.ssh/google_compute_engine.pub`) to your
    [GCP project metatdata](https://cloud.google.com/compute/docs/connect/add-ssh-keys#add_ssh_keys_to_project_metadata).

#### Running tests

```sh
./test/run-e2e-local.sh
```

##### Debugging

Local stderr will show you all GCE calls and SSH commands that were performed by
the test.

###### GCE failures

You may see errors during instance creation. It is likely your service account
is missing permissions.

1.  Make sure the resources are showing up in pantheon under [Compute Engine >
    VM Instances].
2.  Look for errors in pantheon under [Logging > Logs Explorer].
3.  Double check and re-run the
    [service account creation](#create-service-account) step.

###### SSH failures

You may see errors during driver deployment.

1.  Double check that you have an [SSH key file](#create-ssh-key) in the correct
    directory.
2.  `gcloud compute ssh $VM_NAME --zone $ZONE` into the VM instance and attempt
    to execute the last few commands and look for errors.
    -   E.g.
        [vm and driver have a dependency mismatch](https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/1113)).

### Sanity tests

```
./test/run-sanity.sh
```

### Unit tests:

```sh
./test/run-unit.sh
```

### Performance tests

Performance tests are run in automated testing frameworks, but you may wish to
run them locally to benchmark changes. For a given test configuration, you need
to modify the `test-config.yaml` file automatically generated from running one
of the `run-k8s-integration...` or `run-windows-k8s-integration.sh` scripts to
enable performance tests.

```yaml
DriverInfo:
  ...
  PerformanceTestOptions:
    ProvisioningOptions: # deploy 50 1Gi PVCs and measure latency/throughput
      VolumeSize: 1Gi
      Count: 50
      ExpectedMetrics:
        AvgLatency: 10000000000 # 10 seconds
        Throughput: 5
```

You may modify the parameter values to customize the test.

You also need to modify the `StorageClass` file pointed to in `test-config.yaml`
to set `volumeBindingMode: Immediate`, as the performance tests only support
this mode.

We will be running the Kubernetes integration tests directly from its
repository. Install `kubetest` and set up a test cluster with the driver
installed. Now, cd into `$GOPATH/src/k8s.io/kubernetes` and run

```sh
# pwd=k/k
kubetest <custom flags based on your provider> \
  --check-version-skew=false \
  --test \
  --test_args="--ginkgo.focus=External.Storage.*volume-lifecycle-performance --allowed-not-ready-nodes=10 --node-os-distro=<linux or windows> --storage.testdriver=<path-to-test-config>"
```

## Dependency management

Use go modules to manage dependencies

```sh
go mod tidy
```

To modify dependencies or versions change `./Gopkg.toml`

## Debugging

We use https://github.com/go-delve/delve and its feature for remote debugging.
This feature is only available in the PD CSI Controller (which runs in a linux
node).

Requirements:

-   https://github.com/go-delve/delve

Steps:

-   Build the PD CSI driver with additional compiler flags.

```sh
export GCE_PD_CSI_STAGING_VERSION=latest
export GCE_PD_CSI_STAGING_IMAGE=image/repo/gcp-compute-persistent-disk-csi-driver
make build-and-push-multi-arch-debug
```

-   Update `deploy/kubernetes/overlays/noauth-debug/kustomization.yaml` to match
    the repo you wrote above e.g.

```yaml
images:
- name: gke.gcr.io/gcp-compute-persistent-disk-csi-driver
  newName: image/repo/gcp-compute-persistent-disk-csi-driver
  newTag: latest
```

-   Delete and deploy the driver with this overlay

```sh
./deploy/kubernetes/delete-driver.sh && \
  GCE_PD_DRIVER_VERSION=noauth-debug ./deploy/kubernetes/deploy-driver.sh
```

At this point you could verify that delve is running in the controller logs:

```text
API server listening at: [::]:2345
 2021-04-15T18:28:51Z info layer=debugger launching process with args: [/go/src/sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/bin/gce-pd-csi-driver --v=5 --endpoint=unix:/csi/csi.sock]
 2021-04-15T18:28:53Z debug layer=debugger continuing
```

-   Enable port forwading of the PD CSI controller of port 2345

```sh
kubectl -n gce-pd-csi-driver get pods | grep controller | awk '{print $1}' | xargs -I % kubectl -n gce-pd-csi-driver port-forward % 2345:2345
```

-   Connect to the headless server and issue commands

```sh
dlv connect localhost:2345
Type 'help' for list of commands.
(dlv) clearall
(dlv) break pkg/gce-pd-csi-driver/controller.go:509
Breakpoint 1 set at 0x159ba32 for sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-pd-csi-driver.(*GCEControllerServer).ListVolumes() ./pkg/gce-pd-csi-driver/controller.go:509
(dlv) c
> sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-pd-csi-driver.(*GCEControllerServer).ListVolumes() ./pkg/gce-pd-csi-driver/controller.go:509 (hits goroutine(69):1 total:1) (PC: 0x159ba32)
Warning: debugging optimized function
   504:         }
   505: }
   506:
   507: func (gceCS *GCEControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
   508:         // https//cloud.google.com/compute/docs/reference/beta/disks/list
=> 509:         if req.MaxEntries < 0 {
   510:                 return nil, status.Error(codes.InvalidArgument, fmt.Sprintf(
   511:                         "ListVolumes got max entries request %v. GCE only supports values between 0-500", req.MaxEntries))
   512:         }
   513:         var maxEntries int64 = int64(req.MaxEntries)
   514:         if maxEntries > 500 {
(dlv) req
Command failed: command not available
(dlv) p req
*github.com/container-storage-interface/spec/lib/go/csi.ListVolumesRequest {
        MaxEntries: 0,
        StartingToken: "",
        XXX_NoUnkeyedLiteral: struct {} {},
        XXX_unrecognized: []uint8 len: 0, cap: 0, nil,
        XXX_sizecache: 0,}
(dlv)
```

See
https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/pull/742
for the implementation details

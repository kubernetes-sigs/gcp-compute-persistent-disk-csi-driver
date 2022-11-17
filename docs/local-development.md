# Local development
This page contains information on how to develop and test the driver locally.

## Testing

### E2E Tests:

#### One time setup

```console
$ export PROJECT=my-project                               # GCP Project to run tests in
$ export GCE_PD_SA_NAME=$PROJECT-pd-sa                 # Name of the service account to create
$ export GCE_PD_SA_DIR=/my/safe/credentials/directory    # Directory to save the service account key
$ export ENABLE_KMS=false
$ ./deploy/setup-project.sh
```
#### Ongoing runs
```console
$ export PROJECT=my-project                               # GCP Project to run tests in
$ export GCE_PD_SA_NAME=$PROJECT-pd-sa
$ export IAM_NAME=$GCE_PD_SA_NAME@$PROJECT.iam.gserviceaccount.com  # IAM SA that was set up in "one time setup"

$ ./test/run-e2e-local.sh
```

### Sanity Tests
> **_NOTE:_**  Sanity tests are currently failing, tracked by https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver/issues/990.

Sanity tests can be run from VS Code, etc directly or via the cmd line:
```
$ ./test/run-sanity.sh
```

### Unit Tests
Unit tests can be run from VS Code, etc directly or via the cmd line:
```
$ ./test/run-unit.sh
```

## Dependency Management

Use [dep](https://github.com/golang/dep)
```
$ dep ensure
```

To modify dependencies or versions change `./Gopkg.toml`

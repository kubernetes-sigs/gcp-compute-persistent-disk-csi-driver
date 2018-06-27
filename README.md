WARNING: This driver is in ALPHA currently. This means that there may be
potentially backwards compatability breaking changes moving forward. Do NOT use
this drive in a production environment in its current state.

DISCLAIMER: This is not an officially supported Google product

# GCP Compute Persistent Disk CSI Driver

The GCP Compute Persistent Disk CSI Driver is a
[CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md)
Specification compliant driver used by Container Orchestrators to manage the
lifecycle of Google Compute Engine Persistent Disks.

## Installing
### Kubernetes
Templates and further information for installing this driver on Kubernetes are
in [`./deploy/kubernetes/README.md`](deployREADME)

## Development

###Manual

Setup [GCP service account first](deployREADME) (one time step)

To bring up developed drivers:
```
$ make push-container
$ ./deploy/kubernetes/deploy-driver.sh
```

To bring down drivers:
```
$ ./deploy/kubernetes/delete-driver.sh
```

## Testing
Unit tests in `_test` files in the same package as the functions tested.

Sanity and E2E tests can be found in `./test/` and more detailed testing
information is in [`./test/README.md`](testREADME)

## Dependency Management
Use [dep](https://github.com/golang/dep)
```
$ dep ensure
```

To modify dependencies or versions change `./Gopkg.toml`

[deployREADME]: deploy/kubernetes/README.md
[testREADME]: test/README.md

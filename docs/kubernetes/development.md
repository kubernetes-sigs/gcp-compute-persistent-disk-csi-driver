# Kubernetes Development

## Manual

To build and install a development version of the driver:
```
$ GCE_PD_CSI_STAGING_IMAGE=gcr.io/path/to/driver/image:dev   # Location to push dev image to
$ make push-container

# Modify controller.yaml and node.yaml in ./deploy/kubernetes/dev to use dev image
$ GCE_PD_DRIVER_VERSION=dev
$ ./deploy/kubernetes/deploy-driver.sh
```

To bring down driver:
```
$ ./deploy/kubernetes/delete-driver.sh
```

## TODO Testing


# Local development
This page contains information on how to develop and test the driver locally.

## Testing

Running E2E Tests:
```
$ PROJECT=my-project                               # GCP Project to run tests in
$ IAM_NAME=my-iam@project.iam.gserviceaccount.com  # Existing IAM Account with GCE PD CSI Driver Permissions
$ ./test/run-e2e-local.sh
```

Running Sanity Tests:
```
$ ./test/run-sanity.sh
```

Running Unit Tests:
```
$ ./test/run-unit.sh
```

## Dependency Management

Use [dep](https://github.com/golang/dep)
```
$ dep ensure
```

To modify dependencies or versions change `./Gopkg.toml`

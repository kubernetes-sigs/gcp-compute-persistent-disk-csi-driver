# CSI E2E Tests

This package validates the CSI interface using bare GCP nodes (no kubernetes
required).

## Test Tags

These ginkgo tests use tags in square brackets (`[Some Tag]`) in `Describe()` or
`It()` blocks. These are used to group tests to make testing of features easier.

They may be run like

```
PROJECT=$YOUR_PROJECT-gke-dev3 IAM_NAME=$YOUR_SA ./test/run-e2e-local.sh \
  --ginkgo.focus '\[OS-Qualification\]'
```

### Interesting Tags

* `OS-Qualification`: These are a specific subset of tests used for OS image
  qualification. They focus on checking that the OS is set up correctly, rather
  than qualifying the CSI driver generally. Just run without any focus if you're
  qualifying the CSI driver. This can be run with `--instances-per-zone=1`, a
  single zone in `--zones`, and `--machine-type-mw none`.

1. One-time per project: Create GCP service account for CSI driver
    1. Export environment variables for location for service account private key file and name of the service account
    ```
    $ export SA_FILE=~/.../cloud-sa.json
    $ export GCEPD_SA_NAME=sample-service-account
    ```
    2. Setup project with script
    ```
    $ ./deploy/setup_project.sh
    ```
2. Deploy driver to Kubernetes cluster
```
$ ./deploy/kubernetes/deploy_driver.sh
```
3. Create example PVC and Pod
```
$ kubectl apply -f ./examples/demo-pod.yaml
```
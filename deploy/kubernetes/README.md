1. [One-time per project] Create GCP service account credential for CSI Driver and download JSON:
    1. Go to https://pantheon.corp.google.com/apis/credentials
    2. Select `Create credentials` -> ~Service account key`
    3. For `Service Account` select `Compute Engine Default service account`
    4. Select `JSON`
    5. Click `Create` to download JSON for the private key to your local machine.
    6. Rename the downloaded JSON file to `cloud-sa.json`
    6. Export environment variables for 1) location of service account private key JSON file, 2) name you want of service account to be created by `setup_project.sh`, and 3) GCP project:
    ```
    $ export SA_FILE=~/Downloads/cloud-sa.json
    $ export GCEPD_SA_NAME=gcepd-csi-service-account
    $ export PROJECT=my-gcp-project
    ```
    7. Setup project with script
    ```
    $ ./deploy/setup_project.sh
    ```
2. Deploy driver to Kubernetes cluster
```
$ ./deploy/kubernetes/deploy_driver.sh
```
3. Create example workload to verify
    1. Deploy yaml for PVC and Pod
    ```
    $ kubectl apply -f ./examples/demo.yaml
    ```
    2. Verify PV is created and bound to PVC
    ```
    $ kubectl get pvc
    NAME      STATUS    VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
    mypvc     Bound     pvc-e36abf50-84f3-11e8-8538-42010a800002   10Gi       RWO            csi-gce-pd     9s
    ```
    3. Verify pod is created and in `RUNNING` state (it may take a few minutes to get to running state)
    ```
    NAME                      READY     STATUS    RESTARTS   AGE
    sleepypod-xdqqm           1/1       Running   0          1m
    ```

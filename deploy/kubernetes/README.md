Step 1 (Create Credentials):
Create Service Account Credential JSON on GCP:
    - TODO: Add detailed steps on how to do this
    - Requires both Compute Owner and Cloud Project Owner permissions
Create Kubernetes secret:
    -kubectl create secret generic cloud-sa --from-file=cloud-sa.json
Modify "controller.yaml" to use your secret

Step 2 (Set up Driver):
kubectl create -f setup.yaml
kubectl create -f node.yaml
kubectl create -f controller.yaml

Step 3 (Run demo [optional]):
kubectl create -f demo-pod.yaml
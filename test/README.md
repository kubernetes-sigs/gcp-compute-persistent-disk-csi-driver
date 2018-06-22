# How to run Dev tests:

go run test/remote/run_remote/run_remote.go --logtostderr --v 2 --project dyzz-test --zone us-central1-c --ssh-env gce --delete-instances=false --cleanup=false --results-dir=my_test --service-account=${IAM_NAME}
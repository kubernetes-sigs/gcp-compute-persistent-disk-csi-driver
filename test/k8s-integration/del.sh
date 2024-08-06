#!/bin/bash
kubectl delete pod test-pod -n test-ns
kubectl delete pvc test-pvc -n test-ns
kubectl delete vac test-vac1 -n test-ns
kubectl delete vac test-vac2 -n test-ns
kubectl delete sc test-storageclass -n test-ns
kubectl delete ns test-ns

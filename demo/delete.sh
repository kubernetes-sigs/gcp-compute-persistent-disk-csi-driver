#!/bin/bash
kubectl delete pod nginx
kubectl delete sc test-sc-name
kubectl delete pvc test-pv-claim
kubectl delete vac silver

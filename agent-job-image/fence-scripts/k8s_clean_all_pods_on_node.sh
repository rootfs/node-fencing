#!/bin/sh

pods=`kubectl get pods --template '{{range .items}}{{if eq .spec.nodeName "'$1'"}}{{.metadata.name}}{{"\n"}}{{end}}{{end}}'`
echo deleting following pods: $pods
for pod in $pods; do
	kubectl delete pod $pod
done
echo "clean node resource is done"

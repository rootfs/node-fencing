[![Go Report Card](https://goreportcard.com/badge/github.com/rootfs/node-fencing)](https://goreportcard.com/report/github.com/rootfs/node-fencing)

# Kubernetes Node Fence controller
Based on [fence controller design proposal](https://github.com/bronhaim/community/blob/06e367cd5ca391b1a650f2de606875af73604198/contributors/design-proposals/node/node-fence.md)

# Status
pre-alpha

# Goal
The goal is to run controller that monitors partitioned nodes (i.e. not-ready nodes or nodes that raises problem events)
Once partitioned is monitored, the controller posts NodeFence CRD object.

Fence is performed in 3 steps: Isolation, Power-Management and Recovery which are part of the nodeFenceConfig for each node

```yaml
Here we define fence config for node1 which runs "cordon" -> "gcloud-reset-inst" -> "uncordon"

- kind: ConfigMap
  apiVersion: v1
  metadata:
   name: fence-config-node1
   namespace: default
  data:
   config.properties: |-
    node_name=node1
    isolation=cordon
    power_management=gcloud-reset-inst
    recovery=uncordon

The controller moves between steps every grace_period which is defined by the fence-cluster-config

- kind: ConfigMap
  apiVersion: v1
  metadata:
   name: fence-cluster-config
   namespace: default
  data:
   config.properties: |-
    grace_timeout=10
    giveup_retries=5
    roles=
```

# Build
```console
# make
go build -i -o standalone-controller/_output/bin/node-fencing-controller cmd/node-fencing-controller.go
```

# Run
```console
# $ ./standalone-controller/_output/bin/node-fencing-controller -kubeconfig=/home/ybronhaim/.kube/config
  I1228 10:29:01.180885    6376 controller.go:120] Fence controller starting
  I1228 10:29:01.181089    6376 controller.go:123] Waiting for pod informer initial sync
  I1228 10:29:02.181297    6376 controller.go:133] Waiting for node informer initial sync
  I1228 10:29:03.181527    6376 controller.go:144] Waiting for event informer initial sync
  I1228 10:29:04.335665    6376 controller.go:170] Controller monitor is running every 10 seconds
   ...
  W1228 10:34:55.392293    6376 controller.go:272] Node node1-48dw ready status is unknown
  I1228 10:34:55.547233    6376 controller.go:447] Posted NodeFence CRD object for node node1-48dw - starting Isolation
  ...
```

# Deployment in cluster
kubectl create -f standalone-controller/fence-controller-deployment.yaml

# Demo
In https://www.youtube.com/watch?v=l6B7JsAoh50&t we show running example over GCE k8s cluster

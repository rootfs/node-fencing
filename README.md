# Kubernetes Node Fencing Controller

# Status
pre-alpha

# Goal
The goal is to run the controller, filter Pods and find where they are and whether have any PVs. 
If nodes that have PVs on them become offline, the fencing controller tries to force detach or even STONITH the node

# Demo

```console
# make

# _output/bin/node-fencing-controller -kubeconfig=/root/.kube/config 

I1025 14:20:21.175263   23532 controller.go:92] pod controller starting
I1025 14:20:21.175488   23532 controller.go:94] Waiting for informer initial sync
I1025 14:20:22.218596   23532 controller.go:103] node controller starting
I1025 14:20:22.218651   23532 controller.go:105] Waiting for informer initial sync
W1025 14:33:03.903473   24998 controller.go:156] node node063 Ready status is unknown
W1025 14:33:03.903515   24998 controller.go:158] PVs on node node063:
W1025 14:33:03.903544   24998 controller.go:160]        pvc-b9e0e189-b98f-11e7-ae60-00259003b6e8:
I1025 14:33:03.972356   24998 controller.go:294] posted NodeFencing CRD object for node node063
```

On a different console:

```console
# _output/bin/node-fencing-executor -kubeconfig=/root/.kube/config
I1025 14:32:07.029897   24971 executor.go:60] node Fencing executor starting
I1025 14:32:07.030096   24971 executor.go:62] Waiting for informer initial sync
I1025 14:32:08.030295   24971 executor.go:70] Watching node fencing object
I1025 14:33:04.587703   24971 fencing.go:20] fencing output: status :0
I1025 14:33:04.587763   24971 fencing.go:22] fencing node node063 succeeded
```

# Kubernetes Node Fencing Controller

# Status
pre-alpha

# Goal
The goal is to run the controller, filter Pods and find where they are and whether have any PVs. 
If nodes that have PVs on them become offline, the fencing controller tries to force detach or even STONITH the node

# Demo

```console
# make
go build -o _output/bin/node-fencing-controller cmd/node-fencing-controller.go
# _output/bin/node-fencing-controller -kubeconfig=/root/.kube/config 
I1019 15:56:22.153571   22733 controller.go:78] pod controller starting
I1019 15:56:22.153713   22733 controller.go:80] Waiting for informer initial sync
I1019 15:56:23.153962   22733 controller.go:89] node controller starting
I1019 15:56:23.154026   22733 controller.go:91] Waiting for informer initial sync
W1019 15:56:23.209431   22733 controller.go:110] node node1 Ready status is unknown
W1019 15:56:41.295320   22733 controller.go:142] node node2 Ready status is unknown
W1019 15:56:41.295386   22733 controller.go:144] PVs on node node2:
W1019 15:56:41.295411   22733 controller.go:146]     pvc-93f34cb8-b436-11e7-8789-00259003b6e8:
I1019 15:56:42.138674   22733 fencing.go:20] fencing output: status :0
I1019 15:56:42.138723   22733 fencing.go:22] fencing succeeded

```

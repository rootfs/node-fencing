package fencing

import (
	"os/exec"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
)

const (
	defaultFencingScript = "/usr/bin/k8s_node_fencing.sh"
)

func sshFencing(node *v1.Node) {
	addresses := node.Status.Addresses
	for _, addr := range addresses {
		if v1.NodeInternalIP == addr.Type || v1.NodeExternalIP == addr.Type {
			cmd := exec.Command(defaultFencingScript, addr.Address)
			output, err := cmd.CombinedOutput()
			glog.Infof("fencing output: %s", string(output))
			if err == nil {
				glog.Infof("fencing succeeded")
				return
			}
			glog.Infof("fencing to %s failed:%v", addr.Address, err)
		}
	}
}

func Fencing(node *v1.Node, pv *v1.PersistentVolume) {
	if pv.Spec.FC != nil ||
		pv.Spec.ISCSI != nil ||
		pv.Spec.RBD != nil ||
		pv.Spec.NFS != nil ||
		pv.Spec.Glusterfs != nil ||
		pv.Spec.CephFS != nil {
		sshFencing(node)
	}
}

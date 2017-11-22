package fencing

import (
	"os/exec"

	//"fence-executor/providers"
	"github.com/golang/glog"
	//"github.com/rootfs/node-fencing/pkg/fencing/providers"
	"k8s.io/api/core/v1"
	//"time"
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

func FenceNode(node *v1.Node, pv *v1.PersistentVolume) error {
	// search for node fence configmap related to node.Name

	// Create fence providers and agents to execute the fence
	//f := fencing.CreateNewFence()
	//provider := providers.CreateRHProvider(nil)
	//f.RegisterProvider("redhat", provider)
	//err := f.LoadAgents(10 * time.Second)
	//if err != nil {
	//	glog.Infof("error loading agents:", err)
	//	return err
	//}

	// Config agent with parameters related to node

	//ac := fencing.NewAgentConfig(parameters["provider"], parameters["agent"])
	//ac.SetParameter("address", parameters["address"])
	////ac.SetParameter("username", parameters["username"])
	//ac.SetParameter("password", parameters["password"])
	//ac.SetParameter("plug", parameters["port"])

	//err = f.Run(ac, fencing.Status, 10*time.Second)
	//if err != nil {
	//	glog.Infof("error: ", err)
	//	return err
	//}
	glog.Infof("Fenced was executed!")
	return nil
}

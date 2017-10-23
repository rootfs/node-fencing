package main

import (
	"flag"
	"os"
	"os/signal"

	"github.com/golang/glog"

	nfclient "github.com/rootfs/node-fencing/pkg/client"
	"github.com/rootfs/node-fencing/pkg/controller"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "Path to a kubeconfig file")

	flag.Set("logtostderr", "true")
	flag.Parse()

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster
	config, err := buildConfig(*kubeconfig)
	if err != nil {
		panic(err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Failed to create kubernetes client: %v", err)
	}

	aeclient, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	// initialize CRD resource if it does not exist
	err = nfclient.CreateCRD(aeclient)
	if err != nil {
		glog.Fatalf("failed to create CRD: %v", err)
	}

	// make a new config for our extension's API group, using the first config as a baseline
	crdClient, crdScheme, err := nfclient.NewClient(config)
	if err != nil {
		glog.Fatalf("failed to create CRD client: %v", err)
	}

	// wait until CRD gets processed
	err = nfclient.WaitForCRDResource(crdClient)
	if err != nil {
		panic(err)
	}

	ctrl := controller.NewNodeFencingController(client, crdClient, crdScheme)
	stopCh := make(chan struct{})

	go ctrl.Run(stopCh)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	close(stopCh)
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"encoding/base64"
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
	if os.Getenv("SECRET_NAME") == "" {
		glog.Fatal("SECRET_NAME env var is not set")
	}

	secret, err := client.CoreV1().Secrets("default").Get(os.Getenv("SECRET_NAME"), metav1.GetOptions{})
	if err != nil {
		glog.Fatalf("failed to read secret from apiserver: %v", err)
	}

	encPass := base64.StdEncoding.EncodeToString([]byte(secret.Data["password"]))
	passDec, err := base64.StdEncoding.DecodeString(encPass)
	if err != nil {
		glog.Fatalf("error in decoding: %v", err)
	}
	fmt.Printf("%s\n", passDec)
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

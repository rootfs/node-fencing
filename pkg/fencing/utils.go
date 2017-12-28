package fencing

import (
	"github.com/golang/glog"
	"time"
	"strings"
	"errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"syscall"
	"os/exec"
	"k8s.io/client-go/kubernetes"
)

// GetConfigValues returns map with the configName and configType parameters
func GetConfigValues(configName string, configType string, c kubernetes.Interface) map[string]string {
	config, err := c.CoreV1().ConfigMaps("default").Get(configName, metav1.GetOptions{})
	if err != nil {
		glog.Errorf("failed to get %s", err)
		return nil
	}
	properties, _ := config.Data[configType]
	fields := make(map[string]string)

	for _, prop := range strings.Split(properties, "\n") {
		param := strings.Split(prop, "=")
		if len(param) == 2 {
			fields[param[0]] = param[1]
		}
	}
	return fields
}

// WaitTimeout waits for cmd to exist, if time pass pass sigkill signal
func WaitTimeout(cmd *exec.Cmd, timeout time.Duration) (err error) {
	ch := make(chan error)
	go func() {
		ch <- cmd.Wait()
	}()
	timer := time.NewTimer(timeout)
	select {
	case <-timer.C:
		cmd.Process.Signal(syscall.SIGKILL)
		close(ch)
		return errors.New("execute timeout")
	case err = <-ch:
		timer.Stop()
		return err
	}
}

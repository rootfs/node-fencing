package fencing

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

var ErrExecTimeout = errors.New("execute timeout")

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
		return ErrExecTimeout
	case err = <-ch:
		timer.Stop()
		return err
	}
}

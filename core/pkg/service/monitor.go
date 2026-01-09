package service

import (
	"os/exec"
)

type ExitResult struct {
	ExitCode int
	Error    error
}

func startProcessMonitor(cmd *exec.Cmd, exitChan chan<- ExitResult) {
	go func() {
		err := cmd.Wait()

		exitCode := getExitCode(err)

		exitChan <- ExitResult{
			ExitCode: exitCode,
			Error:    err,
		}
	}()
}

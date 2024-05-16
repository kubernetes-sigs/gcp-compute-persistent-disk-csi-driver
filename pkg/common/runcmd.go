package common

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	// Error thrown by exec cmd.Run() when process spawned by cmd.Start() completes before cmd.Wait() is called (see - k/k issue #103753)
	errNoChildProcesses = "wait: no child processes"
)

// RunCommand wraps a k8s exec to deal with the no child process error. Same as exec.CombinedOutput.
// On error, the output is included so callers don't need to echo it again.
func RunCommand(cmd string, args ...string) ([]byte, error) {
	execCmd := exec.Command(cmd, args...)
	output, err := execCmd.CombinedOutput()
	if err != nil {
		if err.Error() == errNoChildProcesses {
			if execCmd.ProcessState.Success() {
				// If the process succeeded, this can be ignored, see k/k issue #103753
				return output, nil
			}
			// Get actual error
			err = &exec.ExitError{ProcessState: execCmd.ProcessState}
		}
		return output, fmt.Errorf("%s %s failed: %w; output: %s", cmd, strings.Join(args, " "), err, string(output))
	}
	return output, nil
}

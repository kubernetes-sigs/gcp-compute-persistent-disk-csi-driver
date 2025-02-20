package common

import (
	"fmt"
	"os/exec"
	"strings"

	"k8s.io/klog/v2"
)

const (
	// Error thrown by exec cmd.Run() when process spawned by cmd.Start() completes before cmd.Wait() is called (see - k/k issue #103753)
	errNoChildProcesses = "wait: no child processes"
)

// RunCommand wraps a k8s exec to deal with the no child process error. Same as exec.CombinedOutput.
// On error, the output is included so callers don't need to echo it again.

func RunCommand(pipeCmd string, pipeCmdArg string, cmd1 string, execCmdArgs ...string) ([]byte, error) {
	execCmd1 := exec.Command(cmd1, execCmdArgs...)

	if pipeCmd != "" {
		output, err := execPipeCommand(pipeCmd, pipeCmdArg, execCmd1)
		if err != nil {
			return nil, fmt.Errorf("%s %s failed here: %w; output: %s", pipeCmd, pipeCmdArg, err, string(output))
		}
		return output, nil
	}
	output, err := execCmd1.CombinedOutput()
	if err != nil {
		err = checkError(err, *execCmd1)
		return nil, fmt.Errorf("%s %s failed here 2: %w; output: %s", cmd1, strings.Join(execCmdArgs, " "), err, string(output))
	}

	return output, nil
}

func checkError(err error, execCmd exec.Cmd) error {
	if err.Error() == errNoChildProcesses {
		if execCmd.ProcessState.Success() {
			// If the process succeeded, this can be ignored, see k/k issue #103753
			return nil
		}
		// Get actual error
		klog.Infof("Errored here")
		err = &exec.ExitError{ProcessState: execCmd.ProcessState}
	}
	return err
}
func execPipeCommand(pipeCmd string, pipeCmdArg string, execCmd1 *exec.Cmd) ([]byte, error) {

	execPipeCmd := exec.Command(pipeCmd, pipeCmdArg)
	stdoutPipe, err := execCmd1.StdoutPipe()
	if err != nil {
		klog.Errorf("failed command %v: got error:%v", execCmd1, err)
	}
	err = execCmd1.Start()
	if err != nil {
		klog.Infof("errored running command %v; error %v; ", execCmd1, err)
	}
	defer stdoutPipe.Close()

	execPipeCmd.Stdin = stdoutPipe
	output, err := execPipeCmd.CombinedOutput()
	if err != nil {
		err = checkError(err, *execPipeCmd)
		return nil, fmt.Errorf("%s failed: %w; output: %s", pipeCmd, err, string(output))
	}

	return output, nil
}

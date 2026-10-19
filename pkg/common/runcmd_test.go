package common

import (
	"strings"
	"testing"
)

func TestRunCommand_UncheckedTypeAssertionPanic(t *testing.T) {
	// Call RunCommand with a pipe destination that does not exist
	// This will cause execPipeCmd.CombinedOutput() to return an *exec.Error,
	// which is NOT an *exec.ExitError. This will trigger the panic.
	_, err := RunCommand("non-existent-command-12345", nil, "echo", "hello")
	if err == nil {
		t.Fatalf("expected error running non-existent pipe command, got nil")
	}

	// We expect the command to return an error, not panic.
	if !strings.Contains(err.Error(), "executable file not found") {
		t.Errorf("expected executable file not found error, got: %v", err)
	}
}

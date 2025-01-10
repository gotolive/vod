package vod

import (
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSuspendProcessReturnsErrorWhenProcessNotFound(t *testing.T) {
	err := suspendProcess(-1)
	assert.Error(t, err)
}

func TestSuspendProcessSucceedsForValidPid(t *testing.T) {
	// Assuming 1 is a valid PID for testing purposes
	var cmd *exec.Cmd
	// TODO windows specific need optimize
	if runtime.GOOS == "windows" {
		cmd = exec.Command("timeout", "/T", "100")
	} else {
		cmd = exec.Command("sleep", "100")
	}

	err := cmd.Start()
	assert.NoError(t, err)
	err = suspendProcess(cmd.Process.Pid)
	assert.NoError(t, err)
	err = resumeProcess(cmd.Process.Pid)
	assert.NoError(t, err)
}

func TestResumeProcessReturnsErrorWhenProcessNotFound(t *testing.T) {
	err := resumeProcess(-1)
	assert.Error(t, err)
}

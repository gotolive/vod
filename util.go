package vod

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// findExecutable find the execute command by the following order:
// 1. Find the command in the environment variable {command}_PATH
// 2. Find the command in the system PATH
// 3. Find the command with .exe suffix in the system PATH on Windows
func findExecutable(command string) string {
	env := strings.ToUpper(command) + "_PATH"
	path := os.Getenv(env)

	if path != "" {
		return path
	}

	if runtime.GOOS == "windows" {
		path, _ = exec.LookPath(command + ".exe")
	} else {
		path, _ = exec.LookPath(command)
	}

	return path
}

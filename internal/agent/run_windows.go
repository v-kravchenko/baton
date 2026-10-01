//go:build windows

package agent

import (
	"errors"
	"os"
	"os/exec"
)

// Run starts the agent as a child process in dir with inherited stdio, waits
// for it and returns its exit code.
func Run(path string, argv []string, dir string) (int, error) {
	cmd := exec.Command(path, argv[1:]...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}

//go:build windows

package agent

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Run starts the agent as a child process in dir with inherited stdio, waits
// for it and returns its exit code.
func Run(path string, argv []string, dir string) (int, error) {
	// A .cmd/.bat shim (npm installs claude.cmd) runs through cmd.exe, which
	// re-parses the command line; Go cannot quote for it (BatBadBut).
	if ext := strings.ToLower(filepath.Ext(path)); ext == ".cmd" || ext == ".bat" {
		for _, a := range argv[1:] {
			if strings.ContainsAny(a, "\"&|<>^%!\r\n") {
				return 1, fmt.Errorf("%s is a batch file; argument %q has characters cmd.exe would interpret", filepath.Base(path), a)
			}
		}
	}
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

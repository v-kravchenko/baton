//go:build !windows

package agent

import (
	"os"
	"strings"
	"syscall"
)

// Run replaces the current process with the agent, started in dir.
func Run(path string, argv []string, dir string) (int, error) {
	if err := os.Chdir(dir); err != nil {
		return 1, err
	}
	env := make([]string, 0, len(os.Environ())+1)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "PWD=") {
			env = append(env, e)
		}
	}
	env = append(env, "PWD="+dir)
	return 1, syscall.Exec(path, argv, env)
}

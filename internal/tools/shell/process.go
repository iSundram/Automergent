package shell

import (
	"fmt"
	"os"
	"os/exec"
)

// Process represents a managed child process.
type Process struct {
	cmd *exec.Cmd
	pid int
}

// Start launches a command as a background process.
func Start(command string, args []string, env []string) (*Process, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), env...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}
	return &Process{cmd: cmd, pid: cmd.Process.Pid}, nil
}

// PID returns the process ID.
func (p *Process) PID() int { return p.pid }

// Kill terminates the process.
func (p *Process) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

// Wait waits for the process to exit.
func (p *Process) Wait() error {
	return p.cmd.Wait()
}

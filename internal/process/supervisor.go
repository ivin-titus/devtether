package process

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// Supervisor controls the lifecycle of child processes routed by DevTether
type Supervisor struct {
	processes map[string]*exec.Cmd
	mu        sync.Mutex
}

// NewSupervisor initializes an empty process tracker
func NewSupervisor() *Supervisor {
	return &Supervisor{
		processes: make(map[string]*exec.Cmd),
	}
}

// StartService springs a new command into a background process, injecting the $PORT variable into its OS environment.
// It intercepts stdout and stderr, prefixing the logs cleanly before sending them to the DevTether stdout.
func (s *Supervisor) StartService(name, command string, assignedPort int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.processes[name]; exists {
		return fmt.Errorf("service '%s' is already running", name)
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Inherit OS environments and inject dynamic PORT
	env := os.Environ()
	env = append(env, fmt.Sprintf("PORT=%d", assignedPort))
	cmd.Env = env

	// Setup stdout/stderr pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to pipe stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to pipe stderr: %w", err)
	}

	// Start Process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command '%s': %w", command, err)
	}

	appLogPrefix := fmt.Sprintf("[%s]", name)
	go streamLog(stdout, appLogPrefix)
	go streamLog(stderr, appLogPrefix)

	s.processes[name] = cmd

	log.Printf("[Supervisor] Started '%s' on port %d (PID: %d)", name, assignedPort, cmd.Process.Pid)

	// Await completion in background to clean up state if process crashes
	go func(n string, c *exec.Cmd) {
		err := c.Wait()
		if err != nil {
			log.Printf("[Supervisor] Service '%s' exited: %v", n, err)
		} else {
			log.Printf("[Supervisor] Service '%s' exited cleanly", n)
		}

		s.mu.Lock()
		delete(s.processes, n)
		s.mu.Unlock()
	}(name, cmd)

	return nil
}

// StopService attempts to gracefully terminate a service by SIGTERM.
func (s *Supervisor) StopService(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cmd, exists := s.processes[name]
	if !exists {
		return fmt.Errorf("service '%s' is not running", name)
	}

	if cmd.Process != nil {
		// Send SIGTERM to the entire process group to prevent ghost orphans
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
			log.Printf("[Supervisor] Failed to SIGTERM '%s' group, attempting SIGKILL: %v", name, err)
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}
	return nil
}

// StopAllServices safely stops all currently running services spawned by this supervisor
func (s *Supervisor) StopAllServices() {
	s.mu.Lock()
	// Create a copy of names to release lock during StopService
	names := make([]string, 0, len(s.processes))
	for name := range s.processes {
		names = append(names, name)
	}
	s.mu.Unlock()

	for _, name := range names {
		// We purposefully ignore errors as we are shutting down universally
		_ = s.StopService(name)
	}
}

func streamLog(pipe io.ReadCloser, prefix string) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		fmt.Printf("%s %s\n", prefix, scanner.Text())
	}
}

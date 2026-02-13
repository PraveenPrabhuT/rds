//go:build unix

package connect

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// startAndWait runs the command in its own process group and gives it the
// terminal so only the child (e.g. pgcli) receives Ctrl+C. This prevents the
// parent RDS CLI from also receiving SIGINT and corrupting the terminal state
// when the user cancels a query, which would cause pgcli to hit termios.error
// when re-entering the prompt.
func startAndWait(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	fd := int(0)
	if term.IsTerminal(fd) {
		// Give terminal to child so only it receives Ctrl+C.
		_ = unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, cmd.Process.Pid)
	}
	err := cmd.Wait()
	if term.IsTerminal(fd) {
		// Restore terminal to parent so the shell works after exit.
		_ = unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, unix.Getpgrp())
	}
	return err
}

//go:build !unix

package connect

import "os/exec"

func startAndWait(cmd *exec.Cmd) error {
	return cmd.Run()
}

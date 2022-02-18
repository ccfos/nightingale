package engine

import "os/exec"

func startCmd(c *exec.Cmd) error {
	return c.Start()
}

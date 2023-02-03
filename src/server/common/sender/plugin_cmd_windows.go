package sender

import "os/exec"

func startCmd(c *exec.Cmd) error {
	return c.Start()
}

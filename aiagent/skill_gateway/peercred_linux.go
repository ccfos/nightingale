//go:build linux

package skillgateway

import (
	"fmt"
	"net"

	"github.com/toolkits/pkg/logger"
	"golang.org/x/sys/unix"
)

// verifyPeer is the SO_PEERCRED defense-in-depth check (§12.1): the per-exec
// socket is bind-mounted into exactly one sandbox, so the only reachable peer is
// already that sandbox's process tree — this is belt-and-suspenders, not the
// primary boundary. user-namespace id mapping makes a strict uid/pid match
// unreliable, so we only confirm a real peer exists and log the creds for audit;
// a probe failure does not fail closed (the bind-mount isolation still holds).
func verifyPeer(c net.Conn) error {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return nil
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return nil
	}
	var ucred *unix.Ucred
	var sErr error
	if cerr := raw.Control(func(fd uintptr) {
		ucred, sErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); cerr != nil || sErr != nil {
		return nil
	}
	if ucred == nil || ucred.Pid <= 0 {
		return fmt.Errorf("rejected gateway peer with invalid credentials")
	}
	logger.Debugf("skill-gateway: peer pid=%d uid=%d gid=%d", ucred.Pid, ucred.Uid, ucred.Gid)
	return nil
}

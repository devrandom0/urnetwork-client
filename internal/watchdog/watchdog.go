// Package watchdog restores system DNS when the VPN process dies without cleaning up, for
// example after SIGKILL: a helper process waits on a pipe that only the VPN holds open.
package watchdog

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

// Subcommand is the hidden first argument that makes the binary run as the watchdog.
const Subcommand = "__dns-watchdog"

// Handle is the VPN's end of the pipe.
type Handle struct {
	w   *os.File
	pid int
}

// Start re-executes this binary as the watchdog for owner, normally the calling process.
func Start(owner int) (*Handle, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd := command(exe, owner, r)
	err = cmd.Start()
	_ = r.Close()
	if err != nil {
		_ = w.Close()
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	return &Handle{w: w, pid: cmd.Process.Pid}, nil
}

// command gives the watchdog an empty environment: it needs nothing from it, and the VPN's
// may still carry URNETWORK_* secrets. Its output goes where this process logs.
func command(exe string, owner int, pipe *os.File) *exec.Cmd {
	cmd := exec.Command(exe, Subcommand, "--owner", strconv.Itoa(owner))
	cmd.Env = []string{}
	cmd.ExtraFiles = []*os.File{pipe}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = detachedProcAttr()
	return cmd
}

// Release tells the watchdog that cleanup is done, so it exits without acting.
func (h *Handle) Release() {
	if h == nil {
		return
	}
	_, _ = h.w.Write([]byte{1})
	_ = h.w.Close()
}

// ignoredSignals: a terminal's Ctrl-C or hangup, or a `pkill urnet-client` meant for the VPN,
// must not take the watchdog down with it.
var ignoredSignals = []os.Signal{os.Interrupt, syscall.SIGHUP, syscall.SIGTERM}

// Main runs the watchdog: args are what follows Subcommand, and the pipe is fd 3.
func Main(args []string, recover func(owner int) error) int {
	fs := flag.NewFlagSet(Subcommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	owner := fs.Int("owner", 0, "")
	if err := fs.Parse(args); err != nil || *owner <= 0 {
		fmt.Fprintln(os.Stderr, "usage: urnet-client "+Subcommand+" --owner=<pid> (started by the VPN, not by hand)")
		return 2
	}
	signal.Ignore(ignoredSignals...)
	pipe := os.NewFile(3, "watchdog-pipe")
	if err := Serve(pipe, *owner, recover); err != nil {
		logx.Error("watchdog: %v\n", err)
		return 1
	}
	return 0
}

// Serve blocks until the VPN releases the watchdog or dies. EOF without the release byte
// means the VPN is gone, since the kernel closes its end of the pipe on any exit.
func Serve(r io.Reader, owner int, recover func(owner int) error) error {
	var b [1]byte
	for {
		n, err := r.Read(b[:])
		if n == 1 {
			return nil
		}
		if err != nil && !errors.Is(err, io.EOF) {
			logx.Warn("watchdog: reading the pipe from pid %d: %v\n", owner, err)
		}
		if err != nil {
			break
		}
	}
	logx.Warn("watchdog: pid %d exited without restoring system DNS; recovering\n", owner)
	return recover(owner)
}

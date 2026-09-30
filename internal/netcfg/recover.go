package netcfg

import (
	"errors"
	"fmt"
)

// RecoverHint ends every error that can leave system DNS pointing at a stub that is gone.
const RecoverHint = "run: sudo urnet-client recover"

// RecoverOptions selects whose leftovers Recover may undo.
type RecoverOptions struct {
	// Owner, when set, limits Recover to state whose DNS marker names this pid: the watchdog
	// passes its parent so it never touches a newer instance's DNS.
	Owner int
	// Force undoes state even while the recorded owner process is alive.
	Force bool
}

// ErrNotOwner means the DNS marker is missing or names another process than RecoverOptions.Owner.
var ErrNotOwner = errors.New("system DNS is not owned by the given process")

// LiveOwnerError reports that a running process other than this one owns system DNS.
type LiveOwnerError struct{ PID int }

func (e *LiveOwnerError) Error() string {
	return fmt.Sprintf("another urnet-client (pid %d) owns system DNS; stop it first", e.PID)
}

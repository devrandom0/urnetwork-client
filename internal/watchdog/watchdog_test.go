package watchdog

import (
	"errors"
	"os"
	"slices"
	"testing"
	"time"
)

// fastRetries makes the recovery retries instant for the duration of a test.
func fastRetries(t *testing.T) {
	t.Helper()
	orig := retryDelays
	retryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryDelays = orig })
}

type recorder struct{ owners []int }

func (r *recorder) recover(owner int) error {
	r.owners = append(r.owners, owner)
	return nil
}

func TestServe_ReleaseByteExitsWithoutRecovery(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	if _, err := w.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	if err := Serve(r, 42, rec.recover); err != nil {
		t.Fatalf("Serve = %v", err)
	}
	if len(rec.owners) != 0 {
		t.Fatal("recovery ran after the parent released the watchdog; DNS would be restored twice")
	}
}

func TestServe_EOFRecoversForTheOwner(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	_ = w.Close()

	if err := Serve(r, 42, rec.recover); err != nil {
		t.Fatalf("Serve = %v", err)
	}
	if !slices.Equal(rec.owners, []int{42}) {
		t.Fatalf("recover calls = %v; want one for pid 42", rec.owners)
	}
}

func TestServe_ReturnsRecoveryErrorAfterThreeAttempts(t *testing.T) {
	fastRetries(t)
	r, w, _ := os.Pipe()
	_ = w.Close()
	want := errors.New("scutil: Permission denied")
	calls := 0

	err := Serve(r, 42, func(int) error { calls++; return want })
	if !errors.Is(err, want) {
		t.Fatalf("Serve = %v; want the recovery error", err)
	}
	if calls != 3 {
		t.Fatalf("recovery attempts = %d; want 3", calls)
	}
}

// Right after a crash scutil or resolvectl can time out once or twice; one failed attempt
// must not leave DNS pointing at the dead VPN's stub.
func TestServe_RetriesATransientRecoveryFailure(t *testing.T) {
	fastRetries(t)
	r, w, _ := os.Pipe()
	_ = w.Close()
	calls := 0

	err := Serve(r, 42, func(int) error {
		calls++
		if calls < 3 {
			return errors.New("scutil timed out after 3s")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("Serve = %v after %d attempts; want success on the third", err, calls)
	}
}

func TestRetryDelays_BoundTheWholeRecovery(t *testing.T) {
	var total time.Duration
	for _, d := range retryDelays {
		total += d
	}
	if len(retryDelays) != 2 || total > 20*time.Second {
		t.Fatalf("retry delays = %v; want 3 attempts within about 20s", retryDelays)
	}
}

func TestMain_RejectsBadOwner(t *testing.T) {
	for _, args := range [][]string{nil, {"--owner", "0"}, {"--owner", "x"}, {"--bogus"}} {
		if code := Main(args, (&recorder{}).recover); code != 2 {
			t.Fatalf("Main(%q) = %d; want 2", args, code)
		}
	}
}

func TestCommand_PassesNoSecretsAndOnlyThePipe(t *testing.T) {
	r, w, _ := os.Pipe()
	defer func() { _ = r.Close(); _ = w.Close() }()

	out, _ := os.CreateTemp(t.TempDir(), "log")
	defer func() { _ = out.Close() }()

	cmd := command("/usr/local/bin/urnet-client", 4242, r, out)

	if !slices.Equal(cmd.Args, []string{"/usr/local/bin/urnet-client", Subcommand, "--owner", "4242"}) {
		t.Fatalf("argv = %q; want only the subcommand and the owner pid", cmd.Args)
	}
	if cmd.Env == nil || len(cmd.Env) != 0 {
		t.Fatalf("env = %q; want an empty environment, not the parent's", cmd.Env)
	}
	if len(cmd.ExtraFiles) != 1 || cmd.ExtraFiles[0] != r {
		t.Fatal("the pipe must be the child's only extra file (fd 3)")
	}
	if cmd.Stderr != out || cmd.Stdout != out {
		t.Fatal("the watchdog must log to the output Start chose for it")
	}
	if none := command("/usr/local/bin/urnet-client", 4242, r, nil); none.Stdout != nil || none.Stderr != nil {
		t.Fatal("no output must leave stdout and stderr unset, which os/exec maps to the null device")
	}
}

func TestRelease_NilHandleIsSafe(t *testing.T) {
	var h *Handle
	h.Release()
}

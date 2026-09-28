package safefile

// SetEUIDForTesting makes the package act as if it runs with the given effective uid, so
// tests in other packages can exercise the root-only rules. Call the returned func to undo it.
func SetEUIDForTesting(uid int) (restore func()) {
	old := geteuid
	geteuid = func() int { return uid }
	return func() { geteuid = old }
}

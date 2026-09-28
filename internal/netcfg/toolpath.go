package netcfg

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// systemToolDirs are searched in order; $PATH is never consulted, so the caller's environment
// cannot substitute the binary a root-run command executes. The last entry is NixOS's system profile.
var systemToolDirs = []string{"/sbin", "/usr/sbin", "/bin", "/usr/bin", "/run/current-system/sw/bin"}

type toolResult struct {
	path string
	err  error
}

type toolLookup struct {
	dirs  []string
	stat  func(string) (fs.FileInfo, error)
	mu    sync.Mutex
	cache map[string]toolResult
}

var tools = &toolLookup{dirs: systemToolDirs, stat: os.Stat, cache: map[string]toolResult{}}

func (l *toolLookup) resolve(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '/') {
		return "", fmt.Errorf("tool name %q must be a bare command name", name)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if r, ok := l.cache[name]; ok {
		return r.path, r.err
	}
	r := toolResult{err: fmt.Errorf("%s not found in %s", name, strings.Join(l.dirs, ", "))}
	for _, d := range l.dirs {
		p := filepath.Join(d, name)
		if fi, err := l.stat(p); err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0 {
			r = toolResult{path: p}
			break
		}
	}
	l.cache[name] = r
	return r.path, r.err
}

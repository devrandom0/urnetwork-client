package netcfg

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// systemToolDirs (per OS) are searched in order; $PATH is never consulted, so the caller's
// environment cannot substitute the binary a root-run command executes.

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
		if l.usable(d, p) {
			r = toolResult{path: p}
			break
		}
	}
	l.cache[name] = r
	return r.path, r.err
}

// usable accepts only an executable that nobody but root could have replaced: the file,
// the search dir and, for a symlink, the target's dir must be root-owned and not group or
// world writable.
func (l *toolLookup) usable(dir, path string) bool {
	fi, err := l.stat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 || !rootOnly(fi) {
		return false
	}
	dirs := []string{dir}
	if resolved, err := filepath.EvalSymlinks(path); err != nil {
		return false
	} else if td := filepath.Dir(resolved); td != dir {
		dirs = append(dirs, td)
	}
	for _, d := range dirs {
		if fi, err := l.stat(d); err != nil || !fi.IsDir() || !rootOnly(fi) {
			return false
		}
	}
	return true
}

func rootOnly(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Uid == 0 && fi.Mode().Perm()&0o022 == 0
}

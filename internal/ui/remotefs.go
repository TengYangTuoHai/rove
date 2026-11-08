package ui

import (
	"sort"
	"strings"
	"time"

	"rove/internal/herdr"
)

// fsEntry is one row of the Files panel: a name, and whether it is a folder.
type fsEntry struct {
	Name string
	Dir  bool
}

// remoteDir is what a folder on a machine said, once.
type remoteDir struct {
	Entries []fsEntry
	Err     string
	Loading bool
	At      time.Time
}

// listScript prints one line per entry: "d" or "f", then the name. It is run
// by the machine's own sh, so it works on macOS and Linux alike.
const listScript = `cd -- "$1" || exit 3
for f in * .[!.]* ..?*; do
  [ -e "$f" ] || [ -L "$f" ] || continue
  if [ -d "$f" ]; then k=d; else k=f; fi
  printf '%s\t%s\n' "$k" "$f"
done`

// remoteListing returns a folder's entries from the cache, asking the
// machine for them the first time (never blocking the frame). Entries are
// looked at again now and then; what is on screen stays until an answer.
func (a *App) remoteListing(on, dir string) remoteDir {
	key := on + "\x00" + dir
	load := false
	a.fsMu.Lock()
	if a.remote == nil {
		a.remote = map[string]*remoteDir{}
	}
	entry := a.remote[key]
	switch {
	case entry == nil:
		entry = &remoteDir{Loading: true, At: time.Now()}
		a.remote[key] = entry
		load = true
	case !entry.Loading && time.Since(entry.At) > 5*time.Second:
		entry.Loading = len(entry.Entries) == 0
		entry.At = time.Now()
		load = true
	}
	cp := *entry
	cp.Entries = append([]fsEntry(nil), entry.Entries...)
	a.fsMu.Unlock()
	if load {
		go a.loadRemoteDir(key, on, dir)
	}
	return cp
}

// loadRemoteDir asks one machine for one folder's entries.
func (a *App) loadRemoteDir(key, on, dir string) {
	res, err := a.S.Exec(on, "", []string{"sh", "-c", listScript, "sh", dir}, 20*time.Second)
	var entries []fsEntry
	errText := ""
	if err != nil {
		errText = err.Error()
	} else if res.Code != nil && *res.Code != 0 {
		if strings.TrimSpace(res.Stderr) != "" {
			errText = strings.TrimSpace(res.Stderr)
		} else {
			errText = "cannot read this folder"
		}
	} else {
		for _, line := range strings.Split(res.Stdout, "\n") {
			if line == "" {
				continue
			}
			kind, name, ok := strings.Cut(line, "\t")
			if !ok || name == "" {
				continue
			}
			entries = append(entries, fsEntry{Name: name, Dir: kind == "d"})
		}
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].Dir != entries[j].Dir {
				return entries[i].Dir
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
	}
	a.fsMu.Lock()
	if a.remote == nil {
		a.remote = map[string]*remoteDir{}
	}
	a.remote[key] = &remoteDir{Entries: entries, Err: errText, At: time.Now()}
	a.fsMu.Unlock()
	a.Repaint()
}

// readRemoteFile reads up to 400 kB of a file on a machine, for the preview.
func (a *App) readRemoteFile(on, path string) []string {
	res, err := a.S.Exec(on, "", []string{"sh", "-c", `head -c 400000 -- "$1"`, "sh", path}, 20*time.Second)
	if err != nil {
		return []string{err.Error()}
	}
	if res.Code != nil && *res.Code != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = "cannot read this file"
		}
		return []string{msg}
	}
	text := res.Stdout
	if strings.ContainsRune(text, 0) {
		return []string{a.tr("Binary file")}
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > 4000 {
		lines = append(lines[:4000], a.tr("… (file truncated)"))
	}
	return lines
}

// machineOfSpace is the machine a pane's folder lives on, "" here.
func (a *App) machineOfSpace() string {
	if sp, _ := a.S.Active(); sp != nil {
		return herdr.SpaceMachine(sp)
	}
	return ""
}

package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"rove/internal/gitcmd"
	"rove/internal/herdr"
)

// filesPanel is the Files panel's state: the folder it shows, the folders
// opened in its tree, and the file it previews.
type filesPanel struct {
	Root    string
	Open    map[string]bool
	File    string
	Content []string
	Err     string
	Loaded  time.Time
}

// changesPanel is the Changes panel's state: git's status and an open diff.
type changesPanel struct {
	Root   string
	On     string
	Files  []changeFile
	Open   string
	Lines  []string
	Loaded time.Time
}

type changeFile struct {
	Status string
	Path   string
	Add    int
	Del    int
}

// panelRoot is the folder the panels show: the focused pane's.
func (a *App) panelRoot() string {
	dir := a.S.FocusedPane().Cwd
	if dir == "" {
		if sp, _ := a.S.Active(); sp != nil {
			dir = sp.Folder
		}
	}
	if dir == "" {
		if on := a.panelMachine(); on != "" {
			return a.S.MachineHome(on)
		}
		dir = homeDir()
	}
	return dir
}

// panelMachine is the machine the panels would read: "" here.
func (a *App) panelMachine() string {
	if sp, _ := a.S.Active(); sp != nil {
		return herdr.SpaceMachine(sp)
	}
	return ""
}

// OpenPanel shows Files or Changes beside the panes.
func (a *App) OpenPanel(id string) {
	if a.panel == id {
		a.panel = ""
		a.Repaint()
		return
	}
	a.panel = id
	a.Repaint()
}

// panelView draws the side panel the tabs bar's buttons show.
func (a *App) panelView(c *ui.Context) {
	switch a.panel {
	case "files":
		a.filesPanelView(c)
	case "changes":
		a.changesPanelView(c)
	}
}

// panelHeader is the bar at the top of a side panel.
func (a *App) panelHeader(c *ui.Context, title, detail string, extra func()) {
	t := a.T
	ui.Column(c).Height(42).FillWidth().Children(func() {
		row := ui.Row(c).Grow(1).FillWidth().PaddingX(14).Gap(8).AlignItems(ui.Center)
		row.Children(func() {
			ui.Text(c, title).FontSize(a.fontSize(13)).FontWeight(600).TextColor(t.Foreground)
			if extra != nil {
				extra()
			}
			ui.Spacer(c)
			if detail != "" {
				ui.Text(c, detail).FontSize(a.fontSize(12)).TextColor(t.Dim).MaxWidth(190).Ellipsis("…").TextAlign(ui.End)
			}
		})
		ui.Box(c).Height(1).FillWidth().Background(t.Border)
	})
}

// filesPanelView is the folder tree and the file preview.
func (a *App) filesPanelView(c *ui.Context) {
	t := a.T
	root := a.panelRoot()
	if a.files.Root != root {
		a.files = filesPanel{Root: root, Open: map[string]bool{}}
	}
	if root == "" {
		ui.Column(c).Fill().Background(t.Background).Radius(10).Border(1, t.Border).Clip().Children(func() {
			a.panelHeader(c, a.tr("Files"), "", nil)
			ui.Text(c, a.tr("Reading the machine's home…")).FontSize(a.fontSize(12)).TextColor(t.Dim).Padding(8)
		})
		return
	}
	ui.Column(c).Fill().Background(t.Background).Radius(10).Border(1, t.Border).Clip().Children(func() {
		detail := root
		if a.files.File != "" {
			detail = a.files.File
		}
		a.panelHeader(c, a.tr("Files"), detail, func() {
			if a.files.File != "" {
				back := ui.Text(c, "←").FontSize(a.fontSize(14)).TextColor(t.Muted)
				if back.Clicked() {
					a.files.File = ""
					a.files.Content = nil
				}
			}
		})
		if a.files.File != "" {
			ui.ScrollBoth(c).Grow(1).Padding(12).Children(func() {
				ui.Column(c).Children(func() {
					for _, line := range a.files.Content {
						ui.Text(c, line).Font("monospace").FontSize(a.fontSize(11.5)).TextColor(t.Foreground).NoWrap()
					}
				})
			})
			return
		}
		ui.Scroll(c).Grow(1).Padding(6).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				a.fileRows(c, root, a.panelMachine(), "", 0)
			})
		})
	})
}

// fileRows draws one folder's rows, and the open ones under them. on is the
// machine the folder is on ("" is this computer).
func (a *App) fileRows(c *ui.Context, root, on, prefix string, depth int) {
	t := a.T
	var entries []fsEntry
	dir := filepath.Join(root, prefix)
	if on == "" {
		list, err := readDir(dir)
		if err != nil {
			ui.Text(c, err.Error()).FontSize(a.fontSize(12)).TextColor(t.Dim).Padding(8)
			return
		}
		for _, e := range list {
			entries = append(entries, fsEntry{Name: e.Name(), Dir: e.IsDir()})
		}
	} else {
		state := a.remoteListing(on, dir)
		if state.Loading {
			ui.Text(c, a.tr("Loading…")).FontSize(a.fontSize(12)).TextColor(t.Dim).Padding(8)
			return
		}
		if state.Err != "" {
			ui.Text(c, state.Err).FontSize(a.fontSize(12)).TextColor(t.Dim).Padding(8).Wrap()
			return
		}
		entries = state.Entries
	}
	for _, e := range entries {
		name := e.Name
		rel := name
		if prefix != "" {
			rel = prefix + "/" + name
		}
		row := ui.Row(c).Height(28).FillWidth().Padding(0, 8, 0, float32(8+depth*18)).Gap(7).Radius(7).AlignItems(ui.Center)
		if row.Hovered() {
			row.Background(t.Hover)
		}
		row.Cursor(ui.CursorPointer)
		row.Children(func() {
			if e.Dir {
				if a.files.Open[rel] {
					a.icon(c, "chevron-down", 12, t.Dim)
				} else {
					a.icon(c, "chevron-right", 12, t.Dim)
				}
			} else {
				ui.Box(c).Width(12)
			}
			if e.Dir {
				a.icon(c, "folder", 14, t.Muted)
			} else {
				a.icon(c, "file", 14, t.Muted)
			}
			ui.Text(c, name).FontSize(a.fontSize(13)).TextColor(t.Foreground).Grow(1).Ellipsis("…")
		})
		if row.Clicked() {
			if e.Dir {
				a.files.Open[rel] = !a.files.Open[rel]
			} else {
				a.openFile(root, on, rel)
			}
		}
		if e.Dir && a.files.Open[rel] {
			a.fileRows(c, root, on, rel, depth+1)
		}
	}
}

// openFile reads a file for the preview, here or on its machine.
func (a *App) openFile(root, on, rel string) {
	var data []byte
	var err error
	if on != "" {
		lines := a.readRemoteFile(on, filepath.Join(root, rel))
		a.files.File = rel
		a.files.Content = lines
		return
	}
	data, err = os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		a.files.Err = err.Error()
		return
	}
	if len(data) > 400_000 {
		data = append(data[:400_000], []byte("\n… (file truncated)")...)
	}
	text := string(data)
	if strings.ContainsRune(text, 0) {
		a.files.File = rel
		a.files.Content = []string{a.tr("Binary file · no text preview")}
		return
	}
	a.files.File = rel
	a.files.Content = strings.Split(text, "\n")
}

// changesPanelView is git's status, the open diff, and the commit box.
func (a *App) changesPanelView(c *ui.Context) {
	t := a.T
	root := a.panelRoot()
	if a.changes.Root != root || time.Since(a.changes.Loaded) > 3*time.Second {
		a.refreshChanges(a.panelMachine(), root)
	}
	ui.Column(c).Fill().Background(t.Background).Radius(10).Border(1, t.Border).Clip().Children(func() {
		add, del := 0, 0
		for _, f := range a.changes.Files {
			add += f.Add
			del += f.Del
		}
		a.panelHeader(c, a.tr("Changes"), "", func() {
			ui.Box(c).PaddingX(5).Radius(4).Border(1, t.Border).Children(func() {
				ui.Text(c, a.tr("%d files", len(a.changes.Files))).FontSize(a.fontSize(10)).TextColor(t.Muted)
			})
			ui.Spacer(c)
			if add > 0 {
				ui.Text(c, "+"+itoa(add)).FontSize(a.fontSize(11)).TextColor(t.Ansi[2])
			}
			if del > 0 {
				ui.Text(c, "−"+itoa(del)).FontSize(a.fontSize(11)).TextColor(t.Ansi[1])
			}
		})
		if len(a.changes.Files) == 0 {
			ui.Box(c).Padding(8).Children(func() {
				ui.Text(c, a.tr("No uncommitted changes")).FontSize(a.fontSize(13)).TextColor(t.Dim)
			})
		}
		ui.Column(c).Height(float32(minInt(200, maxInt(44, len(a.changes.Files)*28+12)))).FillWidth().Children(func() {
			ui.Scroll(c).Grow(1).Padding(6).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					for _, f := range a.changes.Files {
						row := ui.Row(c).Height(28).FillWidth().PaddingX(8).Gap(8).Radius(7).AlignItems(ui.Center)
						if a.changes.Open == f.Path {
							row.Background(t.Selected)
						} else if row.Hovered() {
							row.Background(t.Hover)
						}
						row.Cursor(ui.CursorPointer)
						row.Children(func() {
							color := t.Blocked
							switch f.Status {
							case "A", "U":
								color = t.Ansi[2]
							case "D":
								color = t.Ansi[1]
							}
							ui.Text(c, f.Status).FontSize(a.fontSize(11)).TextColor(color).Width(14)
							dir, name := filepath.Split(f.Path)
							ui.Row(c).Grow(1).Children(func() {
								if dir != "" {
									ui.Text(c, dir).FontSize(a.fontSize(13)).TextColor(t.Dim).Ellipsis("…")
								}
								ui.Text(c, name).FontSize(a.fontSize(13)).TextColor(t.Foreground).MaxWidth(260).Ellipsis("…")
							})
							if f.Add > 0 {
								ui.Text(c, "+"+itoa(f.Add)).FontSize(a.fontSize(11)).TextColor(t.Ansi[2])
							}
							if f.Del > 0 {
								ui.Text(c, "−"+itoa(f.Del)).FontSize(a.fontSize(11)).TextColor(t.Ansi[1])
							}
						})
						if row.Clicked() {
							if a.changes.Open == f.Path {
								a.changes.Open = ""
								a.changes.Lines = nil
							} else {
								a.changes.Open = f.Path
								a.changes.Lines = a.diffLines(root, a.panelMachine(), f.Path)
							}
						}
					}
				})
			})
		})
		ui.Box(c).Height(1).FillWidth().Background(t.Border)
		ui.ScrollBoth(c).Grow(1).Font("monospace").Children(func() {
			ui.Column(c).Padding(6).Children(func() {
				for _, line := range a.changes.Lines {
					row := ui.Row(c).Height(20).FillWidth().Gap(10)
					switch {
					case strings.HasPrefix(line, "+"):
						row.Background(Alpha(t.Ansi[2], 0.10))
					case strings.HasPrefix(line, "-"):
						row.Background(Alpha(t.Ansi[1], 0.10))
					case strings.HasPrefix(line, "@@"):
						row.Background(t.Raised)
					}
					row.Children(func() {
						ui.Text(c, line).Font("monospace").FontSize(a.fontSize(11)).TextColor(t.Foreground).NoWrap()
					})
				}
			})
		})
		ui.Row(c).Height(52).FillWidth().PaddingX(10).PaddingY(10).Gap(8).AlignItems(ui.Center).Children(func() {
			box := ui.Row(c).Height(32).Grow(1).PaddingX(10).Radius(8).Border(1, t.Border).AlignItems(ui.Center)
			box.Children(func() {
				input := ui.TextInputBase(c, &a.commitMessage).FontSize(a.fontSize(12)).TextColor(t.Foreground).Placeholder(a.tr("Commit message…"))
				if input.Submitted() {
					a.commit(a.panelMachine(), root)
				}
			})
			enabled := strings.TrimSpace(a.commitMessage) != "" && len(a.changes.Files) > 0
			btn := ui.Row(c).Height(32).PaddingX(14).Radius(8).AlignItems(ui.Center)
			if enabled {
				btn.Background(t.Solid)
			} else {
				btn.Background(t.Raised)
			}
			btn.Children(func() {
				color := t.Dim
				if enabled {
					color = t.SolidForeground
				}
				ui.Text(c, a.tr("Commit")).FontSize(a.fontSize(12)).TextColor(color)
			})
			if enabled && btn.Clicked() {
				a.commit(a.panelMachine(), root)
			}
		})
	})
}

// gitText runs one git command for a folder, here or on its machine.
func (a *App) gitText(on, root string, args ...string) (string, error) {
	if on == "" {
		out, err := gitcmd.Output(root, args...)
		return string(out), err
	}
	argv := append([]string{"git", "-C", root}, args...)
	res, err := a.S.Exec(on, "", argv, 20*time.Second)
	if err != nil {
		return "", err
	}
	if res.Code != nil && *res.Code != 0 {
		return res.Stdout, exec.ErrNotFound
	}
	return res.Stdout, nil
}

// refreshChanges reads git's status in the folder.
func (a *App) refreshChanges(on, root string) {
	a.changes.Root = root
	a.changes.On = on
	a.changes.Loaded = time.Now()
	out, err := a.gitText(on, root, "status", "--porcelain")
	if err != nil {
		a.changes.Files = nil
		return
	}
	var files []changeFile
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if len(line) < 4 {
			continue
		}
		status := strings.TrimSpace(line[:2])
		if status == "" {
			status = "M"
		}
		path := strings.TrimSpace(line[3:])
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		files = append(files, changeFile{Status: status[:1], Path: path, Add: a.numstat(on, root, path, true), Del: a.numstat(on, root, path, false)})
	}
	a.changes.Files = files
	if a.changes.Open != "" && !hasChange(files, a.changes.Open) {
		a.changes.Open = ""
		a.changes.Lines = nil
	}
}

func hasChange(files []changeFile, path string) bool {
	for _, f := range files {
		if f.Path == path {
			return true
		}
	}
	return false
}

// numstat counts the lines a file adds or deletes.
func (a *App) numstat(on, root, path string, added bool) int {
	out, err := a.gitText(on, root, "diff", "HEAD", "--numstat", "--", path)
	if err != nil {
		return 0
	}
	fields := strings.Fields(out)
	if len(fields) < 2 {
		return 0
	}
	n := 0
	idx := 0
	if !added {
		idx = 1
	}
	for _, r := range fields[idx] {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// diffLines shows a file's diff as git wrote it.
func (a *App) diffLines(root, on, path string) []string {
	out, err := a.gitText(on, root, "diff", "HEAD", "--no-ext-diff", "--no-textconv", "--no-color", "--", path)
	if err != nil || out == "" {
		return []string{a.tr("No text diff available")}
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "@@") {
			return lines[i:]
		}
	}
	return lines
}

// commit stages everything and commits with the message.
func (a *App) commit(on, root string) {
	msg := strings.TrimSpace(a.commitMessage)
	if msg == "" {
		return
	}
	if _, err := a.gitText(on, root, "add", "-A"); err != nil {
		a.toast(a.tr("Commit failed"), err.Error(), "error")
		return
	}
	if _, err := a.gitText(on, root, "commit", "-m", msg); err != nil {
		a.toast(a.tr("Commit failed"), err.Error(), "error")
		return
	}
	a.commitMessage = ""
	a.changes.Loaded = time.Time{}
	a.Repaint()
}

// panelWidth is the side panel's width.
const panelWidth = 368

var _ = herdr.ShortFolder

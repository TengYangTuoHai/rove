package herdr

import (
	"path"
	"sort"
	"strings"
)

// ProgramInfo is what runs in a terminal, as people see it: the name, icon
// and tile of the app's program table (defaults/programs.json).
type ProgramInfo struct {
	Key   string
	Name  string
	Icon  string // a file under programicons/, without the extension
	Tint  bool
	Shell bool
	Agent bool
}

// programEntry is one row of the app's program table (defaults/programs.json).
// Picture marks the full-color logos (`tint: false` there): drawn in their own
// colors, not as a one-color glyph in the text color.
type programEntry struct {
	Name    string
	Icon    string
	Also    []string
	Picture bool
}

var shellNames = []string{"zsh", "bash", "fish", "sh", "dash", "nu", "pwsh", "powershell", "cmd", "elvish", "xonsh", "tcsh", "csh", "ksh", "ion"}

var interpreterNames = []string{"node", "bun", "bunx", "deno", "python", "python3", "npx", "pnpm", "yarn", "uv", "uvx", "ruby", "bundle", "env", "sudo", "nice", "time", "watch", "nodemon"}

var programTable = map[string]programEntry{
	"claude":       {"Claude Code", "claude", []string{"claude-code"}, true},
	"codex":        {"Codex", "codex", nil, true},
	"pi":           {"Pi", "pi", nil, true},
	"pig":          {"PiG", "pig", nil, true},
	"opencode":     {"OpenCode", "opencode", nil, false},
	"cursor":       {"Cursor", "cursor", []string{"cursor-agent"}, false},
	"copilot":      {"Copilot", "copilot", []string{"github-copilot"}, false},
	"amp":          {"Amp", "amp", nil, true},
	"grok":         {"Grok", "grok", []string{"grok-build"}, false},
	"antigravity":  {"Antigravity", "antigravity", []string{"agy"}, true},
	"kimi":         {"Kimi", "kimi", nil, true},
	"kiro":         {"Kiro", "kiro", []string{"kiro-cli"}, true},
	"hermes":       {"Hermes", "hermes", nil, false},
	"mastracode":   {"Mastra Code", "mastracode", nil, false},
	"qoder":        {"Qoder", "qoder", []string{"qodercli"}, false},
	"muse":         {"Muse", "muse", nil, false},
	"crush":        {"Crush", "crush", nil, true},
	"command-code": {"Command Code", "command-code", nil, false},
	"prime-agent":  {"Prime", "prime-agent", nil, false},
	"gemini":       {"Gemini", "gemini", nil, false},
	"ollama":       {"Ollama", "ollama", nil, false},
	// DeepSeek's own CLI (dsh) and its TUI run as dsh-tui.
	"deepseek": {"DeepSeek", "deepseek", []string{"dsh-tui", "dsh", "deepseek"}, true},
	"nvim":     {"Neovim", "nvim", nil, false},
	"vim":      {"Vim", "vim", []string{"vi", "view"}, false},
	"git":      {"Git", "git", nil, false},
	"gh":       {"GitHub", "gh", nil, false},
	"htop":     {"Activity", "htop", []string{"btop", "top", "btm", "glances"}, false},
	"node":     {"Node", "node", nil, false},
	"python":   {"Python", "python", []string{"python3", "ipython", "pip", "pip3"}, false},
	"go":       {"Go", "go", nil, false},
	"cargo":    {"Cargo", "cargo", []string{"rustc", "rustup"}, false},
	"docker":   {"Docker", "docker", nil, false},
	"ssh":      {"SSH", "ssh", []string{"mosh", "et"}, false},
	"tmux":     {"tmux", "tmux", []string{"zellij", "screen"}, false},
	"rg":       {"Search", "rg", []string{"grep", "fzf", "fd", "find"}, false},
	"less":     {"Pager", "less", []string{"more", "man", "bat"}, false},
	"uv":       {"uv", "uv", []string{"uvx"}, false},
	"shell":    {"Shell", "shell", nil, false},
	"program":  {"Program", "program", nil, false},
	"agent":    {"Agent", "agent", nil, false},
}

// agentPrograms are the table's agents, the entries people start (the rows
// with a `run` in programs.json). ProgramInfo.Agent is what a row of the agents
// list is, and what the tabs and pane headers show a logo for.
var agentPrograms = map[string]bool{
	"claude": true, "codex": true, "pi": true, "pig": true,
	"opencode": true, "cursor": true, "copilot": true, "amp": true,
	"grok": true, "antigravity": true, "kimi": true, "kiro": true,
	"hermes": true, "mastracode": true, "qoder": true, "muse": true,
	"crush": true, "command-code": true, "prime-agent": true,
	"gemini": true, "ollama": true, "deepseek": true,
}

// AgentPrograms returns the known agent programs, ordered by display name.
func AgentPrograms() []ProgramInfo {
	var out []ProgramInfo
	for key := range agentPrograms {
		out = append(out, info(key, programTable[key]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// base returns a program's name from a path, without .exe.
func base(s string) string {
	s = path.Base(strings.ReplaceAll(s, "\\", "/"))
	s = strings.TrimLeft(s, "-")
	s = strings.TrimSuffix(s, ".exe")
	return s
}

func inList(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// match finds the table entry for a program or an agent's app name.
func match(name string) (string, programEntry, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if e, ok := programTable[name]; ok {
		return name, e, true
	}
	for key, e := range programTable {
		for _, also := range e.Also {
			if also == name {
				return key, e, true
			}
		}
	}
	return "", programEntry{}, false
}

// info turns a table row into what the window draws: a one-color glyph in the
// text color, or a full-color picture in its own.
func info(key string, e programEntry) ProgramInfo {
	return ProgramInfo{Key: key, Name: e.Name, Icon: e.Icon, Tint: !e.Picture, Agent: agentPrograms[key]}
}

// Program resolves what a terminal runs: what its program says it is, else the
// process in front of its shell, else the program it was started with
// (programs.rs, resolve).
func Program(t *Terminal) ProgramInfo {
	if t == nil {
		return ProgramInfo{Key: "shell", Name: "shell", Icon: "shell", Tint: true, Shell: true}
	}
	// The program's own report wins, even when the table has no logo for it.
	if app := t.activeStatus().app(); app != "" {
		if key, e, ok := match(app); ok {
			return info(key, e)
		}
		return ProgramInfo{Key: "agent", Name: app, Icon: "agent", Tint: true, Agent: true}
	}
	// Else what runs in front of the shell: by its argv (an interpreter shows
	// its script) and then by its name, which may only be a version.
	if fg := t.Foreground; fg != nil {
		if p, ok := fromArgv(fg.Argv); ok {
			return p
		}
		if key, e, ok := match(fg.Name); ok && !inList(interpreterNames, base(fg.Name)) {
			return info(key, e)
		}
	}
	argv := t.Argv
	// The app starts agents in the person's shell; show the agent, not the shell.
	if len(argv) > 4 && inList(shellNames, base(argv[0])) && strings.HasPrefix(argv[1], "-") && strings.HasSuffix(argv[1], "c") {
		argv = argv[4:]
	}
	if p, ok := fromArgv(argv); ok {
		return p
	}
	first := ""
	if len(argv) > 0 {
		first = base(argv[0])
	}
	if first == "" {
		return ProgramInfo{Key: "shell", Name: "shell", Icon: "shell", Tint: true, Shell: true}
	}
	if inList(shellNames, first) {
		return ProgramInfo{Key: "shell", Name: first, Icon: "shell", Tint: true, Shell: true}
	}
	return ProgramInfo{Key: "program", Name: first, Icon: "program", Tint: true}
}

// fromArgv resolves a program's argv: the program it names, or what an
// interpreter among its words runs (node .../vite is Vite).
func fromArgv(argv []string) (ProgramInfo, bool) {
	if len(argv) == 0 {
		return ProgramInfo{}, false
	}
	first := base(argv[0])
	if inList(interpreterNames, first) {
		for _, arg := range argv[1:] {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			name := base(arg)
			for _, ext := range []string{".js", ".mjs", ".cjs", ".ts", ".py", ".rb"} {
				name = strings.TrimSuffix(name, ext)
			}
			if key, e, ok := match(name); ok && !inList(interpreterNames, key) {
				return info(key, e), true
			}
		}
	}
	if key, e, ok := match(first); ok {
		return info(key, e), true
	}
	return ProgramInfo{}, false
}

// runningApps names what runs in the terminal, most telling first: the
// program's own report, then the process in front of its shell by name and by
// what it was called (a versioned binary says "2.1.314", its argv `claude`).
func runningApps(t *Terminal) []string {
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || inList(shellNames, base(name)) || inList(out, name) {
			return
		}
		out = append(out, name)
	}
	add(t.activeStatus().app())
	if fg := t.Foreground; fg != nil {
		add(fg.Name)
		if len(fg.Argv) > 0 {
			add(base(fg.Argv[0]))
		}
	}
	if len(t.Argv) > 0 {
		add(base(t.Argv[0]))
	}
	return out
}

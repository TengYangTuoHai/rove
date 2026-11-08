package herdr

import (
	"io/fs"
	"testing"

	"rove/internal/assets"
)

// Every row of the program table needs its icon: a missing file draws a plain
// terminal glyph instead of the agent's logo.
func TestProgramIconsExist(t *testing.T) {
	for key, e := range programTable {
		if _, err := fs.Stat(assets.Files, "programicons/"+e.Icon+".svg"); err != nil {
			t.Errorf("%s: %v", key, err)
		}
	}
}

// DeepSeek's CLI runs as dsh-tui, and shows its own logo, in its own colors.
func TestProgramFindsDeepSeek(t *testing.T) {
	cases := []*Terminal{
		{Status: &Status{State: "working", App: "dsh-tui"}},
		{Argv: []string{"/opt/homebrew/bin/dsh-tui"}},
		{Argv: []string{"node", "/opt/homebrew/lib/node_modules/@deepseek-harness-tui/dsh-tui/bin/dsh-tui.js"}},
	}
	for _, term := range cases {
		p := Program(term)
		if p.Key != "deepseek" || p.Name != "DeepSeek" || p.Icon != "deepseek" || p.Tint {
			t.Fatalf("%v: %+v", term.Argv, p)
		}
	}
}

// The agents people run resolve to their own logos and colors, from what they
// report about themselves (terminal.get's status.app) as well as from argv.
func TestProgramAgentLogos(t *testing.T) {
	cases := map[string]string{
		"claude-code": "claude", "codex": "codex", "pi": "pi", "opencode": "opencode",
		"grok": "grok", "hermes": "hermes", "gemini": "gemini", "kimi": "kimi",
	}
	for name, icon := range cases {
		p := Program(&Terminal{Status: &Status{State: "idle", App: name}})
		if p.Icon != icon || !p.Agent {
			t.Fatalf("%s: %+v", name, p)
		}
	}
}

// A terminal that runs an agent in the person's shell reads as the agent: its
// status report, its title, or the binary in front of the shell.
func TestProgramAgentInAShell(t *testing.T) {
	// As the live core sends Claude Code: the shell's argv, and the agent's own
	// report in the status; its binary is named after its version.
	claude := &Terminal{
		Argv:       []string{"/bin/zsh", "-l"},
		Title:      "Claude Code",
		Status:     &Status{State: "idle", App: "claude-code"},
		Foreground: &Foreground{Name: "2.1.314", Argv: []string{"claude", "--dangerously-skip-permissions"}},
	}
	if p := Program(claude); p.Key != "claude" || p.Icon != "claude" || p.Tint || !p.Agent {
		t.Fatalf("claude: %+v", p)
	}
	// Without a report, the process in front of the shell still says which.
	quiet := &Terminal{
		Argv:       []string{"/bin/zsh", "-l"},
		Foreground: &Foreground{Name: "2.1.314", Argv: []string{"claude"}},
	}
	if p := Program(quiet); p.Key != "claude" {
		t.Fatalf("quiet claude: %+v", p)
	}
	// A shell alone stays a shell.
	shell := &Terminal{Argv: []string{"/bin/zsh", "-l"}, Foreground: &Foreground{Name: "zsh", Argv: []string{"/bin/zsh", "-l"}}}
	if p := Program(shell); !p.Shell || p.Key != "shell" {
		t.Fatalf("shell: %+v", p)
	}
	// btop in front of a shell is the activity program, as the table says.
	activity := &Terminal{Argv: []string{"/bin/zsh", "-l"}, Foreground: &Foreground{Name: "btop"}}
	if p := Program(activity); p.Key != "htop" {
		t.Fatalf("btop: %+v", p)
	}
}

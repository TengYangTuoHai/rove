package ui

import (
	"slices"
	"testing"

	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

func TestAgentGroupingMenuChangesAndPersistsFromSidebar(t *testing.T) {
	RegisterFonts()
	t.Setenv("ROVE_HOME", t.TempDir())
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.LoadSettings()
	app.setLanguage("en")
	tt := myui.NewTester(app.View, 1280, 820)
	tt.Frame()
	choose := func(label string) {
		t.Helper()
		if err := tt.Click("Group agents"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if !tt.HasText("Show by status") || !tt.HasText("Show by project") {
			t.Fatalf("grouping choices missing: %q", tt.Texts())
		}
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if app.overlay != "" {
			t.Fatalf("selection left overlay %q open", app.overlay)
		}
	}
	if app.settings.AgentsGrouping != agentsByStatus || app.agentGroupingMenu()[0].Icon != "check" {
		t.Fatal("existing status grouping is not the default selection")
	}
	choose("Show by project")
	if app.settings.AgentsGrouping != agentsByProject || app.agentGroupingMenu()[1].Icon != "check" {
		t.Fatal("project grouping did not become selected")
	}
	second := NewApp(nil, ThemeByName("graphite"))
	second.LoadSettings()
	if second.settings.AgentsGrouping != agentsByProject {
		t.Fatal("project grouping did not survive a new launch")
	}
	// The entry remains available with no agents and with the list folded.
	app.toggleAgents()
	tt.Frame()
	choose("Show by status")
	if !app.agentsCollapsed {
		t.Fatal("changing grouping unexpectedly unfolded the list")
	}
	third := NewApp(nil, ThemeByName("graphite"))
	third.LoadSettings()
	if third.settings.AgentsGrouping != agentsByStatus || !third.agentsCollapsed {
		t.Fatalf("status selection or fold did not persist: %+v", third.settings)
	}
}

func TestAgentProjectsUseMachineAndFullWorkingDirectory(t *testing.T) {
	list := []*herdr.Agent{
		{Address: "foreground", Space: "first", SpaceName: "renamed"},
		{Address: "same-cwd", Space: "second", SpaceName: "another name"},
		{Address: "same-name", Space: "first"},
		{Address: "remote", Space: "first", Machine: "gpu"},
		{Address: "space-fallback", Space: "first"},
		{Address: "remote-fallback", Space: "first", Machine: "gpu"},
		{Address: "unknown"},
		{Address: "remote-unknown", Machine: "gpu"},
		{Address: "windows"},
	}
	all := []*herdr.Terminal{
		{Address: "foreground", Cwd: "/shell", Foreground: &herdr.Foreground{Cwd: "/work/rove/"}},
		{Address: "same-cwd", Cwd: "/work/./rove", Foreground: &herdr.Foreground{}},
		{Address: "same-name", Cwd: "/other/rove"},
		{Address: "remote", Cwd: "/work/rove"},
		{Address: "space-fallback"},
		{Address: "windows", Cwd: `C:\work\other`},
	}
	places := []*herdr.Space{
		{ID: "first", Name: "renamed", Folder: "/fallback"},
		{ID: "second", Folder: "/different"},
		{ID: "first", Machine: strp("gpu"), Folder: "/remote-fallback"},
	}
	groups := agentProjects(list, all, places, nil)
	if len(groups) != 8 {
		t.Fatalf("got %d projects, want 8: %+v", len(groups), groups)
	}
	byProject := map[agentProject]agentProjectGroup{}
	for _, group := range groups {
		byProject[group.agentProject] = group
	}
	shared := byProject[agentProject{Folder: "/work/rove"}]
	if len(shared.Agents) != 2 || shared.Label != "/work/rove" {
		t.Fatalf("equal cwd across spaces was not merged/disambiguated: %+v", shared)
	}
	for _, want := range []struct {
		project agentProject
		address string
		label   string
	}{
		{agentProject{Folder: "/other/rove"}, "same-name", "/other/rove"},
		{agentProject{Machine: "gpu", Folder: "/work/rove"}, "remote", "/work/rove"},
		{agentProject{Folder: "/fallback"}, "space-fallback", "fallback"},
		{agentProject{Machine: "gpu", Folder: "/remote-fallback"}, "remote-fallback", "remote-fallback"},
		{agentProject{}, "unknown", ""},
		{agentProject{Machine: "gpu"}, "remote-unknown", ""},
		{agentProject{Folder: "C:/work/other"}, "windows", "other"},
	} {
		group, ok := byProject[want.project]
		if !ok || len(group.Agents) != 1 || group.Agents[0].Address != want.address || group.Label != want.label {
			t.Errorf("project %+v = %+v, want agent %s and label %q", want.project, group, want.address, want.label)
		}
	}
	if groups[len(groups)-1].Folder != "" || groups[len(groups)-2].Folder != "" {
		t.Fatal("unknown projects did not sort last")
	}
	// A live cwd change moves the agent on the next render, without a restart.
	all[1].Cwd = "/new-project"
	groups = agentProjects(list, all, places, nil)
	if len(groups) != 9 {
		t.Fatalf("cwd change did not create a new project: %+v", groups)
	}
}

func TestAgentGroupingKeepsPrioritySubAgentsAndRows(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.setLanguage("en")
	list := []*herdr.Agent{
		{Address: "idle", Title: "idle parent", Status: "idle", Seq: 100},
		{Address: "work-old", Title: "older work", Status: "working", Seq: 1},
		{Address: "work-new", Title: "newer work", Status: "working", Seq: 2},
		{Address: "done", Title: "done parent", Status: "done"},
		{Address: "blocked", Title: "blocked parent", Status: "blocked"},
	}
	original := append([]*herdr.Agent(nil), list...)
	all := []*herdr.Terminal{}
	for _, ag := range list {
		all = append(all, &herdr.Terminal{Address: ag.Address, Cwd: "/work/rove"})
	}
	child := worker("child", "idle", "worker needs approval")
	child.Cwd = "/other/project"
	child.Status = &herdr.Status{State: "blocked"}
	all = append(all, child)
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Width(268).Gap(2).Children(func() {
			app.agentGroups(c, list, all, nil)
		})
	}, 400, 400)
	checkOrder := func(names []string) {
		t.Helper()
		last := float32(-1)
		for _, name := range names {
			rect, ok := tt.Find(name)
			if !ok || rect.Y <= last {
				t.Fatalf("row %q is missing/out of order: %q", name, tt.Texts())
			}
			last = rect.Y
		}
	}
	tt.Frame()
	for _, label := range []string{"Needs you", "Done", "Working"} {
		if !tt.HasText(label) {
			t.Errorf("status section %q missing", label)
		}
	}
	checkOrder([]string{"idle parent", "worker needs approval", "blocked parent", "done parent", "newer work", "older work"})
	app.settings.AgentsGrouping = agentsByProject
	tt.Frame()
	if !tt.HasText("rove") || tt.HasText("Working") || tt.HasText("Done") || tt.HasText("project") {
		t.Fatalf("project headers/sub-agent attachment incorrect: %q", tt.Texts())
	}
	checkOrder([]string{"rove", "idle parent", "worker needs approval", "blocked parent", "done parent", "newer work", "older work"})
	if !slices.Equal(list, original) {
		t.Fatal("grouping mutated the source list")
	}
}

func TestAgentProjectHeadersDisambiguateWithoutWrapping(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.settings.AgentsGrouping = agentsByProject
	folders := []string{"/very/long/first/location/rove", "/very/long/second/location/rove"}
	list := []*herdr.Agent{
		{Address: "first", Title: "first agent", Status: "working"},
		{Address: "second", Title: "second agent", Status: "idle", Machine: "gpu"},
	}
	all := []*herdr.Terminal{{Address: "first", Cwd: folders[0]}, {Address: "second", Cwd: folders[1]}}
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Width(sidebarMinWidth).Gap(2).Children(func() {
			app.agentGroups(c, list, all, nil)
		})
	}, 300, 200)
	tt.Frame()
	for _, folder := range folders {
		r, ok := tt.Find(folder)
		if !ok || r.H > 20 || r.X+r.W > sidebarMinWidth {
			t.Errorf("project header %q missing, wrapped or overflowed: %v", folder, r)
		}
	}
	if !tt.HasText("gpu") || !tt.HasText("first agent") || !tt.HasText("second agent") {
		t.Fatalf("remote identity or agent disappeared: %q", tt.Texts())
	}
}

func TestAgentGroupingSettingsFallbackAndLocalization(t *testing.T) {
	for _, data := range []string{`{}`, `{"agents_grouping":"future"}`, `{"agents_grouping":null}`, `{"agents_grouping":7}`} {
		if got := readSettings([]byte(data), defaultSettings()).AgentsGrouping; got != agentsByStatus {
			t.Errorf("%s selected %q, want status", data, got)
		}
	}
	RegisterFonts()
	for _, tc := range []struct{ language, status, project, unknown string }{
		{"en", "Show by status", "Show by project", "Unknown project"},
		{"zh", "按状态显示", "按项目显示", "未知项目"},
		{"zh-Hant", "按狀態顯示", "按專案顯示", "未知專案"},
		{"ja", "状態別に表示", "プロジェクト別に表示", "不明なプロジェクト"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
			app.systemLanguage = tc.language
			app.setLanguage("system")
			menu := app.agentGroupingMenu()
			if menu[0].Label != tc.status || menu[1].Label != tc.project {
				t.Fatalf("grouping menu not localized: %+v", menu)
			}
			app.settings.AgentsGrouping = agentsByProject
			tt := myui.NewTester(func(c *myui.Context) {
				myui.Column(c).Width(200).Children(func() {
					app.agentGroups(c, []*herdr.Agent{{Address: "missing", Title: "Unknown session", Status: "idle"}}, nil, nil)
				})
			}, 300, 120)
			tt.Frame()
			if !tt.HasText(tc.unknown) || !tt.HasText("Unknown session") {
				t.Fatalf("unknown project lost its localized header or agent: %q", tt.Texts())
			}
		})
	}
}

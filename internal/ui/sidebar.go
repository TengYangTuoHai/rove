package ui

import (
	"path"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

// spaceColors are the avatar colors Herdr gives spaces, by ansi name.
var spaceColors = []string{"ansi6", "ansi3", "ansi4", "ansi5", "ansi2", "ansi1"}

// spaces draws the Spaces list of the sidebar.
func (a *App) spaces(c *ui.Context) {
	t := a.T
	statuses := a.spaceStatuses()
	ui.Column(c).Fill().Background(t.Chrome).Children(func() {
		header := ui.Row(c).Height(31).FillWidth().Padding(0, 10, 0, 16).AlignItems(ui.Center).Justify(ui.SpaceBetween)
		header.Children(func() {
			ui.Text(c, a.tr("Spaces")).FontSize(a.fontSize(11.5)).TextColor(t.Dim)
			a.iconButton(c, "plus", 26, a.tr("New space"), func() { a.newSpace() })
		})
		viewport := ui.Scroll(c).Grow(1).Padding(0, 8, 14, 8)
		scroll := ui.Local(viewport, "sort-scroll", func() ui.ScrollState { return ui.ScrollState{} })
		viewport.TrackScroll(scroll)
		layout := sideReorder{}
		viewport.Children(func() {
			layout = a.spaceItems(c, a.S.Places(), a.S.Machines(), statuses)
		})
		if d, to := layout.update(viewport, scroll); to > 0 {
			switch d.Kind {
			case "space":
				a.S.MoveSpace(d.ID, to)
			case "machine":
				a.S.MoveMachine(d.ID, to)
			}
		}
	})
}

// spaceItems draws the canonical order; sideReorder moves only its visuals.
func (a *App) spaceItems(c *ui.Context, places []*herdr.Space, machines []*herdr.Machine, statuses map[string]string) sideReorder {
	layout := sideReorder{}
	ui.Column(c).FillWidth().Gap(sideRowGap).Children(func() {
		if len(places) == 0 && len(machines) == 0 {
			ui.Row(c).Height(28).PaddingX(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.tr("Loading…")).FontSize(a.fontSize(12)).TextColor(a.T.Dim)
			})
			return
		}
		if len(machines) == 0 {
			list := sideList{}
			for _, sp := range places {
				row := a.spaceRow(c, sp, places, statuses)
				list.add(row, row, sideDrag{Kind: "space", ID: sp.ID, From: herdr.SpaceMachine(sp)}, sp.Index, sideRowHeight)
			}
			layout.spaces = append(layout.spaces, list)
			return
		}
		names := []string{""}
		for _, m := range machines {
			names = append(names, m.Name)
		}
		for i, name := range names {
			group := a.machineGroup(c, name, places, machines, statuses)
			group.spaces.top = layout.groups.nextY
			layout.groups.add(group.element, group.head, sideDrag{Kind: "machine", ID: name}, i, group.height)
			if len(group.spaces.rows) > 0 {
				layout.spaces = append(layout.spaces, group.spaces)
			}
		}
	})
	return layout
}

// spaceStatuses names the most urgent agent of each space, by space id.
func (a *App) spaceStatuses() map[string]string {
	out := map[string]string{}
	for _, ag := range a.S.Agents() {
		if _, ok := out[ag.Space]; !ok {
			out[ag.Space] = ag.Status
		}
	}
	return out
}

// sideDrag is a sidebar row being dragged: a space (Kind "space", From its
// machine's name) or a machine's header (Kind "machine", ID its name).
type sideDrag struct {
	Kind string
	ID   string
	From string
}

// spaceRow is one space: its avatar, name, status dot and the unusual git
// after it.
func (a *App) spaceRow(c *ui.Context, sp *herdr.Space, places []*herdr.Space, statuses map[string]string) ui.Element {
	t := a.T
	spaceName := a.S.SpaceName(sp)
	row := ui.Row(c).Key(sideDrag{Kind: "space", ID: sp.ID}).Height(sideRowHeight).FillWidth().PaddingX(8).Radius(8).AlignItems(ui.Center)
	if sp.Active {
		row.Background(t.Selected)
	} else if row.Hovered() {
		row.Background(t.Hover)
	}
	row.Cursor(ui.CursorPointer)
	row.Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			name := spaceName
			mark := a.spaceMarkFor(sp, places)
			ui.Box(c).Size(14, 14).Radius(4).Background(mark).Center().Children(func() {
				letter := "?"
				if r := []rune(name); len(r) > 0 {
					letter = strings.ToUpper(string(r[0]))
				}
				ui.Text(c, letter).FontSize(a.fontSize(8.5)).FontWeight(700).TextColor(ui.RGB(255, 255, 255))
			})
			// SingleLine keeps a long name on its row's height: it ends in
			// an ellipsis rather than wrapping the row into two.
			ui.Text(c, name).FontSize(a.fontSize(12.5)).TextColor(t.Foreground).MaxWidth(110).
				SingleLine().Shrink(1).MinWidth(0)
			if status := statuses[sp.ID]; status != "" {
				ui.Box(c).Size(6, 6).Radius(3).Background(t.Status(status))
			}
			ui.Spacer(c)
			if tail := a.gitTail(sp, name); tail != "" {
				a.gitChip(c, tail)
			}
		})
	})
	if row.Clicked() {
		a.S.SelectSpace(0, sp.ID)
	}
	// Dragging a row reorders the spaces of its own machine; a space never
	// leaves the machine it runs on.
	machine := herdr.SpaceMachine(sp)
	row.Drag(sideDrag{Kind: "space", ID: sp.ID, From: machine})
	if row.RightClicked() {
		x, y, _ := row.PointerPosition()
		b := row.Bounds()
		id := sp.ID
		a.openMenuAt(b.X+x, b.Y+y, []MenuItem{
			{Label: a.tr("Rename"), Icon: "square-pen", Do: func() {
				a.Prompt(a.tr("Rename space"), a.tr("Leave empty for where its first terminal is"), sp.Name, "space.rename", id)
			}},
			{Label: a.tr("New tab"), Icon: "plus", Do: func() { a.S.SelectSpace(0, id); a.newTab() }},
			{Label: a.tr("Close"), Icon: "close", Do: func() { a.requestClose("space.close", id) }},
		})
	}
	return row
}

// gitChip is a space's unusual git as one compact chip.
func (a *App) gitChip(c *ui.Context, tail string) {
	t := a.T
	ui.Row(c).Height(17).PaddingX(4).Gap(3).Radius(4).Border(1, t.Border).AlignItems(ui.Center).
		Shrink(1).MinWidth(0).Children(func() {
		a.icon(c, "git-branch", 9, t.Dim)
		// A long branch is cut, never wrapped: the chip stays one line high
		// however little room the name leaves it.
		ui.Text(c, tail).FontSize(a.fontSize(10)).Font("JetBrains Mono").TextColor(t.Muted).MaxWidth(92).
			SingleLine().Shrink(1).MinWidth(0)
	})
}

// gitTail is what the row says after the name: repo, branch, ahead, behind.
func (a *App) gitTail(sp *herdr.Space, name string) string {
	info := a.S.GitOf(sp)
	if info.Branch == "" && info.Repo == "" {
		return ""
	}
	branch := info.Branch
	if branch == "HEAD" {
		branch = "detached"
	}
	if strings.EqualFold(branch, name) || branch == "master" || branch == "main" {
		branch = ""
	}
	repo := info.Repo
	if strings.EqualFold(repo, name) {
		repo = ""
	}
	var parts []string
	if repo != "" {
		parts = append(parts, repo)
	}
	if branch != "" {
		parts = append(parts, branch)
	}
	out := strings.Join(parts, " · ")
	if info.Ahead > 0 {
		out += " ↑" + itoa(info.Ahead)
	}
	if info.Behind > 0 {
		out += " ↓" + itoa(info.Behind)
	}
	return out
}

// agents draws the agents list of the sidebar.
func (a *App) agents(c *ui.Context) {
	t := a.T
	list := a.S.Agents()
	ui.Column(c).Fill().Background(t.Chrome).Children(func() {
		blocked, working := 0, 0
		for _, ag := range list {
			switch ag.Status {
			case "blocked":
				blocked++
			case "working":
				working++
			}
		}
		row := ui.Row(c).Height(41).FillWidth().Padding(14, 16, 5, 16).Gap(8).AlignItems(ui.Center)
		row.Children(func() {
			ui.Text(c, a.tr("Agents")).FontSize(a.fontSize(11.5)).FontWeight(500).TextColor(t.Dim)
			ui.Spacer(c)
			a.agentCount(c, "blocked", blocked)
			a.agentCount(c, "working", working)
			a.agentGroupingButton(c)
			chevron := ui.Box(c).Size(20, 20).Radius(4).Center()
			if chevron.Hovered() {
				chevron.Background(t.Hover)
			}
			chevron.Cursor(ui.CursorPointer)
			chevron.Children(func() {
				icon := "chevron-down"
				if a.agentsCollapsed {
					icon = "chevron-right"
				}
				a.icon(c, icon, 12, t.Dim)
			})
			if chevron.Clicked() {
				a.toggleAgents()
				a.Repaint()
			}
		})
		if a.agentsCollapsed {
			return
		}
		if len(list) == 0 {
			ui.Box(c).PaddingX(12).Children(func() {
				ui.Text(c, a.tr("No agent status yet. Older agents or tmux may not report.")).
					FontSize(a.fontSize(12)).TextColor(t.Dim).Wrap()
			})
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().PaddingX(8).Gap(2).Children(func() {
				a.agentGroups(c, list, a.S.Terminals(), a.S.Places())
			})
		})
	})
}

// agentCount is a status dot and count for the agents header.
func (a *App) agentCount(c *ui.Context, word string, n int) {
	if n == 0 {
		return
	}
	t := a.T
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(t.Status(word))
		ui.Text(c, itoa(n)).FontSize(a.fontSize(11)).TextColor(t.Dim)
	})
}

const (
	agentsByStatus  = "status"
	agentsByProject = "project"
)

// agentGroupingButton keeps grouping separate from folding the whole list.
func (a *App) agentGroupingButton(c *ui.Context) {
	button := ui.ButtonBase(c).Size(20, 20).Radius(4).Center().Cursor(ui.CursorPointer).
		Label(a.tr("Group agents")).Tooltip(a.tr("Group agents"))
	if button.Hovered() {
		button.Background(a.T.Hover)
	}
	button.Children(func() {
		icon := "layers"
		if a.settings.AgentsGrouping == agentsByProject {
			icon = "folder"
		}
		a.icon(c, icon, 14, a.T.Dim)
	})
	if button.Clicked() {
		b := button.Bounds()
		a.openMenuAt(b.X, b.Y+b.H, a.agentGroupingMenu())
	}
}

func (a *App) agentGroupingMenu() []MenuItem {
	items := []MenuItem{
		{Label: a.tr("Show by status"), Icon: "layers", Do: func() { a.settings.AgentsGrouping = agentsByStatus; a.Repaint() }},
		{Label: a.tr("Show by project"), Icon: "folder", Do: func() { a.settings.AgentsGrouping = agentsByProject; a.Repaint() }},
	}
	selected := 0
	if a.settings.AgentsGrouping == agentsByProject {
		selected = 1
	}
	items[selected].Icon = "check"
	return items
}

var agentSections = []struct {
	word  string
	label string
}{
	{"blocked", "Needs you"},
	{"done", "Done"},
	{"working", "Working"},
	{"idle", "Idle"},
}

// agentGroups draws status or project sections, keeping each agent's own
// sub-agents under its row.
func (a *App) agentGroups(c *ui.Context, list []*herdr.Agent, all []*herdr.Terminal, places []*herdr.Space) {
	t := a.T
	shown := map[string]bool{}
	for _, ag := range list {
		shown[ag.Address] = true
	}
	// A parent takes its most urgent sub-agent's place in the list.
	section := map[string]string{}
	subs := map[string][]subRow{}
	for _, ag := range list {
		subs[ag.Address] = subAgents(all, shown, ag.Address, nil, 0)
		section[ag.Address] = sectionWord(ag.Status, subs[ag.Address])
	}
	if a.settings.AgentsGrouping == agentsByProject {
		for _, group := range agentProjects(list, all, places, section) {
			label := group.Label
			if group.Folder == "" {
				label = a.tr("Unknown project")
			}
			head := ui.Row(c).Height(30).FillWidth().Padding(8, 8, 0, 8).Gap(6).AlignItems(ui.Center).
				Tooltip(group.Folder)
			head.Children(func() {
				ui.Text(c, label).FontSize(a.fontSize(11)).FontWeight(500).TextColor(t.Dim).
					Shrink(1).MinWidth(0).SingleLine()
				ui.Text(c, itoa(len(group.Agents))).FontSize(a.fontSize(11)).TextColor(t.Dim)
				ui.Spacer(c)
				if group.Machine != "" {
					a.machineChip(c, group.Machine)
				}
			})
			for _, ag := range group.Agents {
				a.agentRow(c, ag)
				for _, sub := range subs[ag.Address] {
					a.subAgentRow(c, sub)
				}
			}
		}
		return
	}
	for _, sec := range agentSections {
		var mine []*herdr.Agent
		for _, ag := range list {
			if section[ag.Address] == sec.word {
				mine = append(mine, ag)
			}
		}
		if len(mine) == 0 {
			continue
		}
		sort.SliceStable(mine, func(i, j int) bool { return mine[i].Seq > mine[j].Seq })
		head := ui.Row(c).Height(30).FillWidth().Padding(8, 8, 0, 8).Gap(6).AlignItems(ui.Center)
		head.Children(func() {
			ui.Text(c, a.tr(sec.label)).FontSize(a.fontSize(11)).FontWeight(500).TextColor(t.Status(sec.word))
			ui.Text(c, itoa(len(mine))).FontSize(a.fontSize(11)).TextColor(t.Dim)
			ui.Spacer(c)
		})
		for _, ag := range mine {
			a.agentRow(c, ag)
			for _, sub := range subs[ag.Address] {
				a.subAgentRow(c, sub)
			}
		}
	}
}

// agentProject identifies a working directory on one machine. Names of spaces
// and repositories are presentation only: neither identifies a project.
type agentProject struct {
	Machine string
	Folder  string
}

type agentProjectGroup struct {
	agentProject
	Label  string
	Agents []*herdr.Agent
}

// agentProjects resolves the same cwd as a pane (foreground, terminal, space),
// without filesystem access: a remote path belongs to its remote machine.
func agentProjects(list []*herdr.Agent, all []*herdr.Terminal, places []*herdr.Space, sections map[string]string) []agentProjectGroup {
	terminals := make(map[string]*herdr.Terminal, len(all))
	for _, term := range all {
		terminals[term.Address] = term
	}
	spaceFolders := make(map[[2]string]string, len(places))
	for _, sp := range places {
		spaceFolders[[2]string{herdr.SpaceMachine(sp), sp.ID}] = sp.Folder
	}
	groups := []agentProjectGroup{}
	indices := map[agentProject]int{}
	for _, ag := range list {
		dir := ""
		if term := terminals[ag.Address]; term != nil {
			if term.Foreground != nil {
				dir = term.Foreground.Cwd
			}
			if dir == "" {
				dir = term.Cwd
			}
		}
		if dir == "" {
			dir = spaceFolders[[2]string{ag.Machine, ag.Space}]
		}
		if dir != "" {
			// Windows cwd may also arrive from a machine running Windows.
			if (len(dir) > 2 && dir[1] == ':' && (dir[2] == '\\' || dir[2] == '/')) || strings.HasPrefix(dir, `\\`) {
				dir = strings.ReplaceAll(dir, `\`, "/")
			}
			dir = path.Clean(dir)
		}
		project := agentProject{Machine: ag.Machine, Folder: dir}
		at, ok := indices[project]
		if !ok {
			at = len(groups)
			indices[project] = at
			label := ""
			if dir != "" {
				label = path.Base(dir)
			}
			groups = append(groups, agentProjectGroup{agentProject: project, Label: label})
		}
		groups[at].Agents = append(groups[at].Agents, ag)
	}
	// A basename is compact until it becomes ambiguous; then show the full
	// path. A remote machine also has its own chip in the project header.
	names := map[string]int{}
	for _, group := range groups {
		names[group.Label]++
	}
	for i := range groups {
		group := &groups[i]
		if names[group.Label] > 1 {
			group.Label = group.Folder
		}
		sort.SliceStable(group.Agents, func(i, j int) bool {
			a, b := group.Agents[i], group.Agents[j]
			aw, bw := a.Status, b.Status
			if word := sections[a.Address]; word != "" {
				aw = word
			}
			if word := sections[b.Address]; word != "" {
				bw = word
			}
			if ar, br := herdr.StatusRank(aw), herdr.StatusRank(bw); ar != br {
				return ar > br
			}
			return a.Seq > b.Seq
		})
	}
	// Project headers stay put as statuses change; unavailable cwd goes last.
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if (a.Folder == "") != (b.Folder == "") {
			return a.Folder != ""
		}
		if an, bn := strings.ToLower(a.Label), strings.ToLower(b.Label); an != bn {
			return an < bn
		}
		if a.Machine != b.Machine {
			return a.Machine < b.Machine
		}
		return a.Folder < b.Folder
	})
	return groups
}

// subRow is a sub-agent under its parent's row, with the tree lines in front
// of it.
type subRow struct {
	Term  *herdr.Terminal
	Lines []string
}

// subWord is a sub-agent's word as the list shows it: an error reads as
// blocked, and done as idle — a sub-agent reports to its parent, and the
// parent's done is the news (as Herdr's agents list does).
func subWord(word string) string {
	switch word {
	case "error":
		return "blocked"
	case "done":
		return "idle"
	}
	return word
}

// sectionWord is the section a row is drawn in: its own word, or the more
// urgent word of a sub-agent under it.
func sectionWord(word string, subs []subRow) string {
	for _, sub := range subs {
		if w := subWord(sub.Term.Word()); herdr.StatusRank(w) > herdr.StatusRank(word) {
			word = w
		}
	}
	return word
}

// subAgents lists the live terminals an agent started, depth first, each with
// the lines that lead to it: a worker started from an agent's terminal has no
// pane of its own, and Herdr lists it under its parent. A sub-agent a pane
// already shows is a row of its own, so only its children are added there.
func subAgents(all []*herdr.Terminal, shown map[string]bool, parent string, lead []string, depth int) []subRow {
	if depth > 8 {
		return nil
	}
	var kids []*herdr.Terminal
	for _, term := range all {
		if term.State != "live" || term.Parent == nil || *term.Parent != parent {
			continue
		}
		kids = append(kids, term)
	}
	var out []subRow
	for i, term := range kids {
		line, next := "├", "│"
		if i == len(kids)-1 {
			line, next = "└", ""
		}
		lines := append(append([]string{}, lead...), line)
		if !shown[term.Address] {
			out = append(out, subRow{Term: term, Lines: lines})
		}
		out = append(out, subAgents(all, shown, term.Address, append(append([]string{}, lead...), next), depth+1)...)
	}
	return out
}

// subAgentRow is a sub-agent under the agent that started it: indented, with
// the tree lines that lead to it, its program's logo and its name. It has no
// pane, so a click looks at it over the tab, as Herdr's agents list does.
func (a *App) subAgentRow(c *ui.Context, sub subRow) {
	t := a.T
	term := sub.Term
	row := ui.Row(c).Height(26).FillWidth().Padding(0, 8, 0, 14).Radius(8).AlignItems(ui.Center)
	if a.overlay == "peek" && a.peek.addr == term.Address {
		row.Background(t.Selected)
	} else if row.Hovered() {
		row.Background(t.Hover)
	}
	row.Cursor(ui.CursorPointer)
	row.Children(func() {
		for _, line := range sub.Lines {
			ui.Box(c).Width(11).Height(26).Center().Children(func() {
				ui.Text(c, line).FontSize(a.fontSize(12)).TextColor(t.Input)
			})
		}
		ui.Box(c).Width(8)
		a.programIcon(c, herdr.Program(term), 14, t.Foreground)
		ui.Box(c).Width(8)
		ui.Text(c, terminalName(term, term.Address)).FontSize(a.fontSize(12.5)).TextColor(t.Muted).Grow(1).MinWidth(0).SingleLine()
		a.agentStatus(c, subWord(term.Word()))
	})
	if row.Clicked() {
		a.peekTerminal(term.Address)
	}
}

// agentRow is one agent: the same 26 px row, radius 8 and 2 px gap as the
// spaces above it, so the two lists share one hover surface.
func (a *App) agentRow(c *ui.Context, ag *herdr.Agent) {
	t := a.T
	row := ui.Row(c).Height(26).FillWidth().PaddingX(8).Gap(8).Radius(8).AlignItems(ui.Center)
	if ag.Focused {
		row.Background(t.Selected)
	} else if row.Hovered() {
		row.Background(t.Hover)
	}
	row.Cursor(ui.CursorPointer)
	row.Children(func() {
		a.programIcon(c, ag.Program, 14, t.Foreground)
		name := ag.Title
		if name == "" {
			name = ag.SpaceName
			if !strings.HasPrefix(ag.TabName, ag.SpaceName) && ag.TabName != "" && ag.TabName != "1" {
				name += " · " + ag.TabName
			}
		}
		color := t.Foreground
		if ag.Status == "idle" && !ag.Focused {
			color = t.Muted
		}
		ui.Text(c, name).FontSize(a.fontSize(12.5)).TextColor(color).Grow(1).MinWidth(0).SingleLine()
		// Project headers already identify the machine for all their agents.
		if ag.Machine != "" && a.settings.AgentsGrouping != agentsByProject {
			a.machineChip(c, ag.Machine)
		}
		a.agentStatus(c, ag.Status)
	})
	if row.Clicked() {
		a.S.FocusPane(ag.Pane)
	}
}

// machineChip is the machine an agent runs on, as one compact chip the
// height of a space's git chip. An agent on this computer has none.
func (a *App) machineChip(c *ui.Context, name string) {
	t := a.T
	ui.Row(c).Height(17).PaddingX(4).Gap(3).Radius(4).Border(1, t.Border).AlignItems(ui.Center).
		Shrink(1).MinWidth(0).Children(func() {
		a.icon(c, "server", 9, t.Dim)
		ui.Text(c, name).FontSize(a.fontSize(10)).TextColor(t.Muted).MaxWidth(92).
			SingleLine().Shrink(1).MinWidth(0)
	})
}

// agentStatus is the working spinner, the Needs you pill, the idle ring or
// the done dot.
func (a *App) agentStatus(c *ui.Context, word string) {
	t := a.T
	switch word {
	case "working":
		a.spinner(c, 12, t.Working)
	case "blocked":
		ui.Box(c).Height(18).PaddingX(6).Radius(5).Center().Background(Alpha(t.Blocked, 0.15)).Children(func() {
			ui.Text(c, a.tr("Needs you")).FontSize(a.fontSize(10.5)).FontWeight(600).TextColor(t.Blocked)
		})
	case "idle":
		ui.Box(c).Size(7, 7).Radius(4).Border(1.5, t.Idle)
	case "done":
		ui.Box(c).Size(7, 7).Radius(4).Background(t.Done)
	}
}

// newSpace starts a space in the focused folder, on the shown machine.
func (a *App) newSpace() {
	sp, _ := a.S.Active()
	on := ""
	if sp != nil && sp.Machine != nil {
		on = *sp.Machine
	}
	a.newSpaceOn(on)
}

// newSpaceOn starts a space on one machine ("" is this computer).
func (a *App) newSpaceOn(machine string) {
	go func() {
		if _, _, err := a.S.NewSpace("", "", machine, nil); err != nil {
			a.toast(a.tr("Cannot start a space"), err.Error(), "error")
		}
	}()
}

// itoa is fmt.Sprint for small positive numbers.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// spaceMarkFor is a space's avatar color: the one fewest spaces use, as
// Herdr's mods agree on (a space called "~" keeps the first).
func (a *App) spaceMarkFor(sp *herdr.Space, places []*herdr.Space) ui.Color {
	assigned := map[string]string{}
	for _, other := range places {
		// A space called ~ is not given a color, as in Herdr's spaces mod.
		if other.Name == "~" {
			if other.ID == sp.ID {
				return ui.Hex(markColor(spaceColors[0]))
			}
			continue
		}
		uses := func(c string) int {
			n := 0
			for _, v := range assigned {
				if v == c {
					n++
				}
			}
			return n
		}
		best := spaceColors[0]
		for _, c := range spaceColors {
			if uses(c) < uses(best) {
				best = c
			}
		}
		if other.ID == sp.ID {
			return ui.Hex(markColor(best))
		}
		assigned[other.ID] = best
	}
	return ui.Hex(markColor(spaceColors[0]))
}

// markColor is the strong color a space avatar shows for an ansi name.
func markColor(name string) string {
	switch name {
	case "ansi6":
		return "#22a99a"
	case "ansi3":
		return "#e3814f"
	case "ansi4":
		return "#5b8def"
	case "ansi5":
		return "#8b7cf6"
	case "ansi2":
		return "#3fae6a"
	case "ansi1":
		return "#d9649a"
	}
	return "#22a99a"
}

// machineGroup draws one machine's spaces under its header, once a second
// machine is known.
func (a *App) machineGroup(c *ui.Context, name string, places []*herdr.Space, machines []*herdr.Machine, statuses map[string]string) sideGroup {
	t := a.T
	var mine []*herdr.Space
	for _, sp := range places {
		if herdr.SpaceMachine(sp) == name {
			mine = append(mine, sp)
		}
	}
	var info *herdr.Machine
	for _, m := range machines {
		if m.Name == name {
			info = m
		}
	}
	connected := name == "" || (info != nil && info.Connected())
	// The dot says whether the machine answers, in the colors the Machines
	// settings page gives the same states. Idle's grey is Dim's own grey, so a
	// connected machine used to look no different from a machine away.
	state := "asleep"
	if name == "" {
		state = "connected"
	}
	if info != nil && info.State != "" {
		state = info.State
	}
	folded := a.foldedMachines[name]
	group := sideGroup{height: sideRowHeight, spaces: sideList{nextY: sideRowHeight + sideRowGap}}
	group.element = ui.Column(c).Key(sideDrag{Kind: "machine", ID: name}).FillWidth().Gap(sideRowGap)
	group.element.Children(func() {
		head := ui.Row(c).Key("header").Height(sideRowHeight).FillWidth().PaddingX(8).Gap(7).Radius(8).AlignItems(ui.Center)
		group.head = head
		head.Cursor(ui.CursorPointer)
		var plus ui.Element
		head.Children(func() {
			icon := "monitor"
			if name != "" {
				icon = "server"
			}
			a.icon(c, icon, 13, t.Dim)
			label := name
			if label == "" {
				label = a.tr("This computer")
			}
			ui.Text(c, label).FontSize(a.fontSize(11)).TextColor(t.Dim).Grow(1).Ellipsis("…")
			if !connected {
				ui.Text(c, a.tr(machineStateText(state))).FontSize(a.fontSize(11)).TextColor(t.Dim)
			} else {
				plus = ui.Box(c).Size(16, 16).Radius(4).Center()
				if plus.Hovered() {
					plus.Background(t.Hover)
				}
				plus.Cursor(ui.CursorPointer)
				plus.Children(func() { a.icon(c, "plus", 11, t.Muted) })
				if plus.Clicked() {
					a.newSpaceOn(name)
				}
			}
			ui.Box(c).Size(6, 6).Radius(3).Background(a.machineStateColor(state))
			chevron := "chevron-down"
			if folded {
				chevron = "chevron-right"
			}
			a.icon(c, chevron, 11, t.Dim)
		})
		// The plus is its own hover surface; light the header only outside it, so
		// the two never stack into a brighter patch.
		if head.Hovered() && !(connected && plus.Hovered()) {
			head.Background(t.Hover)
		}
		if head.Clicked() {
			a.toggleMachine(name)
		}
		// The header drags the whole group; the viewport owns its drop target.
		if name != "" {
			head.Drag(sideDrag{Kind: "machine", ID: name})
		}
		if folded {
			return
		}
		if len(mine) == 0 {
			if !a.S.MachineReady(name) {
				if !connected {
					return
				}
				group.height += sideRowGap + 28
				ui.Row(c).Height(28).PaddingX(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.tr("Loading…")).FontSize(a.fontSize(12)).TextColor(t.Dim)
				})
				return
			}
			group.height += sideRowGap + sideRowHeight
			row := ui.Row(c).Height(sideRowHeight).PaddingX(8).Gap(7).AlignItems(ui.Center).Radius(8)
			if row.Hovered() {
				row.Background(t.Hover)
			}
			row.Cursor(ui.CursorPointer)
			row.Children(func() {
				a.icon(c, "plus", 12, t.Dim)
				ui.Text(c, a.tr("New space there")).FontSize(a.fontSize(12)).TextColor(t.Dim)
			})
			if row.Clicked() {
				a.newSpaceOn(name)
			}
			return
		}
		for _, sp := range mine {
			row := a.spaceRow(c, sp, places, statuses)
			group.spaces.add(row, row, sideDrag{Kind: "space", ID: sp.ID, From: name}, sp.Index, sideRowHeight)
			group.height += sideRowGap + sideRowHeight
		}
	})
	return group
}

// toggleAgents folds the agents list to its header, or unfolds it, and keeps
// the fold for the next launch.
func (a *App) toggleAgents() {
	a.agentsCollapsed = !a.agentsCollapsed
	a.settings.AgentsCollapsed = a.agentsCollapsed
}

// toggleMachine folds a machine's spaces under its header, or unfolds them,
// and keeps the fold for the next launch.
func (a *App) toggleMachine(name string) {
	if a.foldedMachines[name] {
		delete(a.foldedMachines, name)
	} else {
		a.foldedMachines[name] = true
	}
	a.settings.FoldedMachines = a.foldedMachines
}

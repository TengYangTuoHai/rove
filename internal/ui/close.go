package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

type closeState struct {
	kind  string
	names []string
	run   func()
}

// requestClose keeps the original target while the person decides, even if
// another window changes the active tab in the shared document meanwhile.
func (a *App) requestClose(command, target string) {
	if a.overlay == "close-confirm" {
		return
	}
	spaces := a.S.Places()
	for _, sp := range spaces {
		for _, tab := range sp.Tabs {
			tab.Root = a.S.Tree(tab.ID)
		}
	}
	st := closeState{
		kind:  strings.TrimSuffix(command, ".close"),
		names: closeWarnings(command, target, spaces, a.S.Terminal),
		run:   func() { a.closeTarget(command, target) },
	}
	if len(st.names) == 0 {
		st.run()
		return
	}
	a.closing = st
	a.OpenOverlay("close-confirm")
}

func closeWarnings(command, target string, spaces []*herdr.Space, terminal func(string) *herdr.Terminal) []string {
	var names []string
	seen := map[string]bool{}
	var visit func(*herdr.Node)
	visit = func(n *herdr.Node) {
		if n == nil {
			return
		}
		if command == "pane.close" && n.ID != target {
			for _, child := range n.Children {
				visit(child)
			}
			return
		}
		if n.Content != nil {
			addr := n.Content.Terminal
			if addr != "" && !seen[addr] {
				seen[addr] = true
				if term := terminal(addr); term.RunningCommand() {
					name := herdr.Program(term).Name
					if fg := term.Foreground; fg != nil && fg.Name != "" {
						name = fg.Name
					}
					names = append(names, name)
				}
			}
		}
		for _, child := range n.Children {
			visit(child)
		}
	}
	for _, sp := range spaces {
		if command == "space.close" && sp.ID != target {
			continue
		}
		for _, tab := range sp.Tabs {
			if command == "tab.close" && tab.ID != target {
				continue
			}
			visit(tab.Root)
		}
	}
	return names
}

func (a *App) closeTarget(command, target string) {
	switch command {
	case "pane.close":
		a.S.ClosePane(target)
	case "tab.close":
		a.S.CloseTab(target)
	case "space.close":
		a.S.CloseSpace(target)
	}
}

func (a *App) confirmClose() {
	st := a.closing
	a.CloseOverlay()
	if st.run != nil {
		st.run()
	}
}

func (a *App) closeConfirmOverlay(c *ui.Context) {
	t := a.T
	st := a.closing
	width := min(float32(440), a.W-32)
	panel := a.card(c, width, 210).Absolute().Top((a.H - 210) / 2).Left((a.W - width) / 2).Padding(20).Gap(12)
	panel.Children(func() {
		ui.Text(c, a.tr("Close %s?", a.tr(st.kind))).FontSize(a.fontSize(15)).FontWeight(600).TextColor(t.Foreground)
		detail := a.tr("A program is still running in this terminal. Closing will end it.")
		if len(st.names) > 1 {
			detail = a.tr("Programs are still running in %d terminals. Closing this %s will end them.", len(st.names), a.tr(st.kind))
		}
		ui.Text(c, detail).FontSize(a.fontSize(12.5)).TextColor(t.Muted).Wrap()
		ui.Text(c, strings.Join(st.names, ", ")).FontSize(a.fontSize(12)).TextColor(t.Dim).MaxWidth(width - 40).Ellipsis("…")
		ui.Spacer(c)
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			cancel := ui.Button(c, a.tr("Cancel")).AutoFocus()
			if cancel.Clicked() {
				a.CloseOverlay()
			}
			close := ui.PrimaryButton(c, a.tr("Close %s", a.tr(st.kind)))
			if close.Clicked() {
				a.confirmClose()
			}
		})
	})
}

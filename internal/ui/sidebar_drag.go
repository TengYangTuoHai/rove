package ui

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	sideRowHeight float32 = 26
	sideRowGap    float32 = 2
)

var sideRowMotion = ui.Animation{Spring: ui.SpringOptions{Duration: 220 * time.Millisecond}}

// sideList keeps the canonical slots separate from the animated rows. Its top
// is in scroll-content coordinates; each row's y is relative to its parent.
// Transforms never change these slots, so letting a row move out from under
// the pointer cannot change the insertion position again.
type sideList struct {
	top, nextY float32
	rows       []sideRow
}

type sideRow struct {
	element, source ui.Element
	value           sideDrag
	index           int // the 1-based position the session accepts
	y, height       float32
}

func (l *sideList) add(element, source ui.Element, value sideDrag, index int, height float32) {
	l.rows = append(l.rows, sideRow{element, source, value, index, l.nextY, height})
	l.nextY += height + sideRowGap
}

// destination counts the rows whose original midpoints the pointer crossed,
// excluding the source. Index-zero rows (the local machine) stay fixed.
func (l sideList) destination(d sideDrag, y float32) (int, bool) {
	from, to := -1, 0
	for i, row := range l.rows {
		if row.value == d && row.index > 0 {
			from = i
		}
	}
	if from < 0 {
		return 0, false
	}
	for i, row := range l.rows {
		if row.index == 0 {
			if y < l.top+row.y+row.height+sideRowGap {
				return 0, false
			}
			to++
		} else if i != from && y > l.top+row.y+row.height/2 {
			to++
		}
	}
	if d.Kind == "space" {
		// While machine metadata is loading the sidebar can be flat. Keep
		// the same-machine constraint even before group headers are known.
		for _, row := range l.rows[min(from, to) : max(from, to)+1] {
			if row.value.From != d.From {
				return 0, false
			}
		}
	}
	return to, true
}

func (l sideList) animate(d sideDrag, to int, preview bool) {
	order := l.rows
	if preview {
		order = slices.Clone(l.rows)
		from := slices.IndexFunc(order, func(row sideRow) bool { return row.value == d })
		row := order[from]
		order = slices.Delete(order, from, from+1)
		order = slices.Insert(order, to, row)
	}
	y := float32(0)
	if len(l.rows) > 0 {
		y = l.rows[0].y
	}
	for _, row := range order {
		// Animate absolute parent coordinates. When a drop changes the real
		// order, the new layout baseline is subtracted immediately while the
		// keyed animation retains its current position and velocity.
		shown := row.element.AnimateTo("sidebar-y", y, sideRowMotion)
		row.element.Translate(0, shown-row.y)
		if row.source.Dragging() {
			// MyGo paints the drag copy independently of the source's opacity.
			row.element.Opacity(0)
		}
		y += row.height + sideRowGap
	}
}

type sideGroup struct {
	element, head ui.Element
	height        float32
	spaces        sideList
}

type sideReorder struct {
	groups sideList
	spaces []sideList
}

type sideRelease struct {
	y     float32
	valid bool
}

// update gives the entire viewport one drop target, including gaps and empty
// space below the rows. A nonzero result commits a confirmed, allowed move.
func (layout sideReorder) update(viewport ui.Element, scroll *ui.ScrollState) (sideDrag, int) {
	release := ui.Local(viewport, "sort-release", func() sideRelease { return sideRelease{} })
	viewport.ObserveInput(func(ev ui.InputEvent) {
		if ev.Button != 0 {
			return
		}
		switch ev.Kind {
		case ui.InputPointerDown:
			release.valid = false
		case ui.InputPointerUp:
			// Drop exposes the payload, not the release coordinates. Capture
			// them before the pointer or the scroll offset can change again.
			release.y, release.valid = ev.Y+scroll.Y, !ev.Canceled
		}
	})
	_, y, _ := viewport.PointerPosition()
	y += scroll.Y
	d, over := ui.DragOver[sideDrag](viewport)
	dropped, drop := ui.Drop[sideDrag](viewport)
	if drop {
		d, y, over = dropped, release.y, release.valid
		release.valid = false
	}

	move := 0
	to, valid := 0, false
	if over && d.Kind == "machine" {
		to, valid = layout.groups.destination(d, y)
	}
	layout.groups.animate(d, to, valid)
	if drop && valid {
		if row := layout.groups.rows[to]; row.value != d {
			move = row.index
		}
	}
	for _, list := range layout.spaces {
		to, valid = 0, false
		// A space stays on its machine. Its header accepts the first slot;
		// only the final section extends into the viewport's empty tail.
		bottom := list.top + list.nextY
		last := bottom >= layout.groups.nextY
		if over && d.Kind == "space" && y >= list.top && (y < bottom || last) {
			to, valid = list.destination(d, y)
		}
		list.animate(d, to, valid)
		if drop && valid && list.rows[to].value != d {
			move = list.rows[to].index
		}
	}
	return d, move
}

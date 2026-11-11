# Herdr's UI, measured

A study of [herdr-app](https://github.com/herdrdev/herdr-app) (Herdr 2,
`herdr-app 0.1.11`, herdr-core 0.1.16) made to rebuild its window 1:1 in Go
with MyGo native UI. Everything here was read from the app's Rust and
JavaScript sources, its shipped `defaults/`, and screenshots taken with
`herdr-app shot` (2× pixels for a 1280×820 window).

## Two programs

- **herdr-core (`hc`)** runs every terminal in a PTY and keeps them alive
  after the window closes. It also stores the shared world, as a versioned
  document per machine: `herdr-app`. Terminals carry labels (`herdr.space`,
  `herdr.tab`, `herdr.pinned`, `herdr.seen`, `herdr.right-click`); a
  terminal with the labels shows up in the app even when started elsewhere.
- **herdr-app** is the window: spaces, tabs, pane splits, focus, the agents
  list, panels, dialogs. It is a GPUI (Rust) app whose panels are JavaScript
  "mods" drawn through `gpui-kit`, and whose window comes from a layout JSON.

Model:

```
space (id sp…, name, folder, machine?, tabs[])
  tab (id tb…, name, root: layout node tree)
    block (id p…, content {terminal: "t7"} or {mod, props})
    row/column (children with size {px|weight})
view per screen: active space, each space's tab, each tab's focus/zoom
```

A tab's stored root is a row/column tree of blocks; splitting right on a
single-pane tab turns its root into `row` with two weight-0.5 blocks (ids
of panes are fresh `p…`).

Agents are terminals whose program reported through OSC 7501
(`working`, `blocked`, `done`, `idle`, `error`). Attention order:
blocked > done > working > idle. `done` becomes `idle` once looked at.

## The window: `layouts/main.json`

```json
{ "type": "row", "titlebar": "none", "traffic_lights": [12, 16],
  "background": "canvas", "terminal_padding": [14, 12],
  "divider": { "width": 1, "color": "border" },
  "children": [
    { "id": "sidebar", "size": {"px": 268}, "min": 44, "max": 480,
      "background": "chrome", "resizable": false, "children": [
        { "id": "brand", "size": {"px": 46}, "window_drag": true, "content": {"mod": "workspace-chrome"} },
        { "id": "sidebar-lists", "children": [
          { "id": "spaces", "size": {"weight": 1}, "min": 90, "content": {"mod": "spaces"} },
          { "id": "agents", "size": {"weight": 1}, "min": 90, "content": {"mod": "agents"} } ] },
        { "id": "sidebar-footer", "size": {"px": 69}, "content": {"mod": "workspace-chrome", "props": {"footer": true}} } ] },
    { "id": "work", "children": [
      { "id": "tabs", "size": {"px": 42}, "window_drag": true, "content": {"mod": "tabs"} },
      { "id": "work-body", "type": "row", "padding": 8, "gap": 8, "children": [
        { "id": "main", "content": { "tab": "active", "pane": {
            "gap": 8, "radius": 10, "background": "background", "lone": true,
            "border": {"width": 1, "color": "border"},
            "focus": {"width": 1, "color": "accent"},
            "header": {"content": {"mod": "pane"}, "height": 30, "position": "top", "reveal": "always"} } } },
        { "id": "panel", "type": "column", "hidden": true, "size": {"px": 368}, "min": 280, "max": 900,
          "children": [ { "id": "panel-body", "type": "stack", "active": "files",
            "children": [ {"id": "files", "content": {"mod": "files"}},
                          {"id": "changes", "content": {"mod": "changes"}} ] } ] } ] },
      { "id": "status", "size": {"px": 28}, "content": {"mod": "workspace-status"} } ] } ] }
```

Measured from screenshots (logical px at 1280×820, shot is 2×):

| edge | x / y |
|---|---|
| sidebar | 0–268, `chrome` `#101114` |
| sidebar divider | 268–269, `border` |
| brand row | y 0–46 |
| spaces header | y 46–77 (`h31`, `pl16 pr10`) |
| spaces list | px 8, rows h26, 2 px apart |
| agents section | starts y 398.5 (border-top 1 px), header `h41 pt14 pb5 px16` |
| footer | y 751–820 (border-top 1 px, px8 pt4 pb8, rows h28) |
| tab bar | y 0–42, `chrome`, border-bottom 1 px, `px10 gap3` |
| tab pill | h29, `px10 gap7 radius7 border1`, active `raised` + `border` |
| work body | padding 8, gap 8, `canvas` `#131518` |
| pane card | radius 10, border 1 `border` (1 `accent` while focused) |
| pane header | h30, `px10 gap8`, bg `background`, border-bottom 1 `border`, text 12 |
| pane gap | 8 |
| status bar | h28, `chrome`, border-top 1, `px14 gap16`, text 11.8 |
| side panel | 368 wide, gap 8 |

Split panes divide the available space by their weights; a lone pane fills
the tab. Zoom shows one pane full size. The macOS traffic lights sit at
(12, 16), over the brand row; the brand leaves them 76 px.

## Theme: graphite, resolved

A palette becomes every color the app draws (`settings.rs::resolve`):

```
background  #17191d      foreground  #eeeeef
chrome      mix(bg, 0, 0.32)         canvas   mix(bg, 0, 0.16)
popover     mix(bg, #fff, 0.06)      raised   mix(bg, #fff, 0.09)
hover       #ffffff0e    selected  #ffffff17
border      #ffffff18    input     #ffffff24
muted       mix(bg, fg, 0.6)         dim      mix(bg, fg, 0.38)
accent      ansi4        accent_foreground = bg
solid       fg           solid_foreground = mix(bg, 0, 0.28)
ring        mix(bg, fg, 0.7)         selection #394454
working     #e5c07b      blocked   #f0705f     done  #76a8ff
idle/unknown mix(bg, fg, 0.38)
```

The graphite ANSI palette: `#45484f #dc8c95 #a6bd91 #d8be87 #96b2d7
#b5a2ce #8cbdc3 #d2d4d9 #818793 #f0a5ad #bad5a2 #ead2a1 #b0cbef #cbb7e6
#a4d5db #f0f0f2`; terminal foreground `#d2d4d9`, selection `#394454`.

Light themes (`paper`) derive chrome toward warm grey. `ink`, `moss` are
other dark palettes. Type: terminals JetBrains Mono 14 px (shipped as a
variable TTF), panels Geist 13 px (size 11–13.5 in mods), terminal padding
14×12, terminal opacity 1.

## Components

**Brand** (`workspace-chrome`): h46, `chrome`, `pl76` on macOS, gap 4/8: logo
20×20 radius 5, "Herdr" 13.5 semibold; right: search 26×26 (opens picker),
sidebar toggle 26×26 (collapses the sidebar to 76 on macOS, 44 elsewhere).
Below 160 px the sidebar is a rail: no labels, numbered spaces, agents as
numbers + status, footer icon only.

**Spaces** (`spaces`): header 31 px, "Spaces" 11.5 dim, + 26×26. Rows h26
`px8 radius8`, 2 px apart: 14×14 radius-4 avatar in a space color with the
first letter (8.5 bold white), name 12.5 cut to one line, and only unusual
git at the right as a chip (h17 `px4 gap3 radius4 border1`: branch 9, repo ·
branch ↑ahead ↓behind 10 mono cut to one line, skipping the name,
master/main).
Selected uses `selected`, hover `hover`; drag reorders (accent line at the
drop edge) among the spaces of the same machine, and a space dropped on its
machine's header goes to the top of that list; right-click menu Rename / New
worktree / Open worktree / Close. Machines with spaces, or with more than one
machine known, get a header h26 `px8 gap7` (server 13, name 11 dim, + 16×16,
state dot 6, chevron 11) that folds its list; dragging a remote header moves
its group up or down, and the order is kept in `view.json`'s `machines`.
Worktrees indent under their checkout with `├─`/`└─`. Empty/unknown:
"Loading…" h28.

**Agents** (`agents`): border-top 1; header 41 (`pt14 pb5 px16`), "Agents"
11.5 medium dim, grouping icon 14 (Show by status, Show by project), plus a
separate fold chevron. Rove keeps the grouping in `settings.json` as
`agents_grouping` (`status`, the default, or `project`). Projects use the
agent's foreground cwd, then terminal cwd, then space folder; a machine and
full cleaned path identify one project across spaces. Equal directory names
show full paths, remote groups name their machine, and unavailable cwd is
"Unknown project". Projects sort by name; rows retain attention order and
newest-turn order, with sub-agents staying under their parent. Status
sections "Needs you" / "Done" / "Working" / "Idle": headers h30 `px8 pt8`
11 medium colored by status with a count; rows h26 `px8 gap8 radius8` 2 px
apart, the height and inset of a space's row: program icon 14,
"space · detail" 12.5 (idle muted), machine chip when the agent is on
another machine (h17 `px4 gap3 radius4 border1`, server 9, name 10 muted
cut to one line; this computer's agents have none), status at right:
spinner 12 working, `Needs you` pill (h18 px6 radius5, blocked at 15 %
alpha), 7 px ring idle, 7 px dot done. Empty text 12 dim:
"No agent status yet. Agents show up here when they report their status.
Older versions and agents inside tmux may not."

**Footer**: border-top; rows h28 `px8 gap9 radius8`: gear 14 + "Settings"
13 (muted), + 14 + "Add machine" 13 (dim); hover bg and fg.

**Tabs bar** (`tabs`): `px10`, left padding leaves the traffic lights when
the sidebar is hidden (`max(10, (12+64)/zoom − sidebar width)`), gap 3.
Pills h29 `px10 gap7 radius7 border1`: program icon 14, name 12.8
(unnamed tabs show their number; " Z" when zoomed), status (spinner 12 /
6 px dot), close ×12 on the active tab. Double-click renames, right-click
shows Rename / New tab / Close, drag moves. Then + 26 (new-tab menu),
Spacer, Files and Changes buttons `h26 px8 gap5 radius6 text12` with icon
14, active `selected`. Wheel over the bar switches tabs.

**Pane card** (`pane` header mod): header h30 `px10 gap8`: program icon 14,
title 12 medium (truncated at 260), folder 12 dim (`~` at home), status
(spinner 12 working / `Needs you` / 7 px done dot), and on hover four
buttons `p3 radius4` (split right, split down, zoom/unzoom, close) at 14.
Click focuses the pane; right-click gives the pane menu.

**Status bar** (`workspace-status`): 11.8 muted `px14 gap16`: monitor 13 +
"This computer" (or server + machine), branch icon + branch when any,
6 px dim dot + "N agents" + "· M needs you" in blocked; Spacer; palette
hint at 11.8 dim = the bound key prettified ("⌘⇧P  commands").

**Files** (`files`, panel or overlay): radius 10, border, bg background;
header h42 `px14 gap8` border-bottom: back arrow when previewing, "Files"
semibold 13, folder/file name 12 dim right. Tree: rows h28 `pl(8+depth*18)
pr8 gap7 radius7`, chevron 12, folder/file icon 14, name 13, git mark at
right (A green, D/U red, M blocked). File preview: monospace 12 with
syntax colors. Limit 500 entries per folder, 5,000 total.

**Changes** (`changes`): header h42 with "Changes", a `N files` badge
(10 px, border), +add / −del at 11. File list rows h28 `px8 gap8 radius7`:
status letter (A/D/U/M) colored, path + name 13, +/− counts 11; list max
200 px. Diff: monospace 11, rows h20 `px12 gap10`, `+`/`−` tinted with
ansi green/red at ~10 %, hunk headers on `raised`. Commit box: input h32
radius8 border (accent while typing) and a Commit button h32 px14 radius8
(`solid` when enabled), border-top.

**Command palette** (`frame/palette`): popup 620 wide, 85 % of the window
high, at top offset 60, padding 18, gap 10, bg popover, radius 12, border,
shadow. Search row h36 `px10 gap10 radius9 border1 accent`, bg background:
command icon 16 dim, input 12.8, "Esc" 11 dim. Rows h32 `mx6 px10 gap10
radius7 text12.8`: title, group 11.5 dim while searching, key chips
`px5 radius4 border1 raised text11` — raw parts, one chip each ("cmd",
"shift", "]"). Group headers h26 `px14 pb4 text11 semibold`. Selected
`selected`, hover `hover`. ↑/↓ move, Page Up/Down by 10, Enter runs,
Esc closes; fuzzy score favours word starts and runs; empty shows recents
then groups Tab, Pane, Space, Layout, Terminal, Agent, Window, App.

**Picker** (go to space/tab): same popup family, spaces with something
that needs you first, "earlier" folded, type to filter, Enter goes,
Ctrl+D closes a tab.

**New-tab menu**: popup 250 wide at the pointer, min(491, 167 + 31·rows)
high, padding 5: Terminal (with its key), "Start an agent" and the
installed agents (1–9 starts them), divider, New space, New worktree…,
Open worktree….

**Context menus**: popup 200 wide, rows h30 `px8 gap8 radius6` with a
14 px icon: pane (Split right, Split down, Zoom, Send right-clicks…, Send
back to parent, Close pane); tab (Rename, New tab, Close); space (Rename,
New worktree, Open worktree, Close); global (New space, Keybinds, Themes).

**Welcome** (`frame/welcome`): 660×600 window overlay, header h64 `px24`:
logo 26, "Welcome to Herdr" 17 semibold, BETA badge 10, "Drag to move" 12
dim, ×; body `px32` gap 14: title 21 semibold, paragraph 13.5 muted, three
features (34×34 radius8 `raised` icon tile, 13.5 semibold title, 12.5 dim
text), divider, Appearance segmented (System/Light/Dark), Size stepper
(− 100% +), pages dots and "Next →" (solid button). 4 pages; it sets
`frame.welcomed`.

**Settings**: 1080 wide, 720 high, centered: left column 224 `chrome`
(padding 8/10/12, groups App/Herdr over the sections Look/Terminal and
Machines/Alerts/Updates/Privacy, rows h34 `px10 gap10 radius8` 2 px apart so
their surfaces never merge, selected `selected`, hover `hover`; version row
at the foot behind a 1 px border), 1 px border, right pane
`p24 28` gap 16: section title 18, rows of label 13 + about 11.5 dim on
the left and controls on the right (segmented, theme swatches 92×48,
fonts, zoom stepper).

**Toasts**: bottom right, width 320, padding 12, radius 10, popover, border,
shadow; title 12.5 semibold (level color), body 11.5 muted.

**Peek**: `terminal.peek` opens a terminal over the tab (a worker without a
pane, or Cmd-click on an address in terminal text) with keep-as-tab and
close.

## Interactions

Keys come from `keys.json` (sections `global`, `layout:<name>`, `terminal`,
`mod:<name>`; the more specific wins). The shipped macOS set:

| key | action | | key | action |
|---|---|---|---|---|
| ⌘Q | app.quit | | ⌘D | pane.split:right |
| ⌘H | app.hide | | ⌘⇧D | pane.split:down |
| ⌘T | tab.new | | ⌘W | pane.close |
| ⌘⇧W | tab.close | | ⌘⇧Enter | pane.zoom |
| ⌃Tab / ⌃⇧Tab | tab.next / prev | | ⌘⌥←↑→↓ | pane.go |
| ⌘⇧] / ⌘⇧[ | tab.next / prev | | ⌘⌥⇧←↑→↓ | pane.swap |
| ⌘⌥] / ⌘⌥[ | space.next / prev | | F2 | tab.rename |
| ⌘] / ⌘[ | agent.next / prev | | ⇧F2 | space.rename |
| ⌘⇧L | layout.switch | | ⌘/ | app.help |
| ⌘N | space.new | | ⌘B | layout.toggle:sidebar |
| ⌘P | mod.open:picker | | ⌘= ⌘- ⌘0 | zoom in/out/reset |
| ⌘⇧P | app.palette | | ⌘1…⌘9 | tab.select:1…9 |
| ⌘, | app.settings | | ⌃1…⌃9 | space.select:1…9 |

Terminal section: ⌘C/⌘V copy/paste, ⌃V paste image, ⌥←/→/⌫ and ⌘←/→/⌫
send readline escapes. Plain Ctrl+letters go to the program.

Mouse: click a space to select; click a tab to select; double-click a tab to
rename; right-click for menus; drag a space to reorder it, or a machine's
header to move its group; drag a tab to move;
drag the sidebar edge to resize (44–480); drag the panes' top-center grip to
swap or place panes (Escape cancels); drag the brand or tab bar to move the
window. In terminals: Ghostty's selection, Option-drag rectangle, Cmd-click
links and terminal addresses, wheel scrollback (or the program's own mouse),
Cmd+C/V, right-click Herdr's menu or the program's.

Overlays: dialog dims the window and keeps the keyboard; popup closes on a
click beside it; window stays until closed, drags by its background, keeps
its spot. Escape closes the top one. The welcome is a window; the palette
and menus are popups at the pointer or the top.

## Protocol (herdr-core)

`hc connect` → one line `hello {framed:true}` → frames
`[channel u32 LE][kind u8][len u32 LE][payload]`, kinds DATA 0, CREDIT 1,
CLOSE 2, RESIZE 3, FOCUS 4, FACTS 5. Channel 0 carries JSON-RPC.

- `terminal.attach {terminal, cols, rows, history:2000, display}` replies
  `{channel, terminal}`; the channel first sends a paint of the current
  screen (and scrollback), then the program's output. Keys are DATA on the
  same channel; RESIZE/FOCUS/FACTS tell the core the viewer's grid, focus
  and colors; CREDIT returns flow control (256 KiB start, 1 MiB for
  events); CLOSE detaches.
- `events.subscribe` returns `{channel, seq, terminals}`; notifications
  (`created`, `lost`, `restarted`, `finished`, `status`, `turn`, `title`,
  `labels`, `size`, `foreground`, `keep`, `removed`, `doc`, `missed`,
  heartbeat) update the records; `missed` re-reads a snapshot.
- The world is `doc.get/doc.set` name `herdr-app`, versioned, conflict on a
  stale write.
- App commands are app-side: create a terminal (`terminal.run` with labels
  and `keep:true`), then write the document; close kills the terminal.

## Where this port puts it

| Original | Here |
|---|---|
| `src/hc.rs` | `internal/core/client.go` |
| `src/model.rs` | `internal/herdr/model.go`, `session.go` |
| `src/mirror.rs` document | same, JSON in and out |
| `src/settings.rs` theme | `internal/ui/theme.go` |
| `src/programs.rs` | `internal/herdr/program.go` |
| `layouts/main.json` + frame | `internal/ui/app.go` (`sidebar`, `work`, `paneCard`, `statusBar`) |
| `mods/spaces`, `mods/agents` | `internal/ui/sidebar.go` |
| `mods/tabs`, `mods/pane`, `mods/workspace-chrome/status` | `internal/ui/app.go` |
| `mods/files`, `mods/changes` | `internal/ui/panel.go` |
| `frame/palette`, `mods/picker`, `mods/new-menu`, `mods/menu` | `internal/ui/overlays.go` |
| `frame/welcome`, `frame/settings`, `mods/help` | `internal/ui/overlays.go` |
| `src/terminal.rs` | `internal/herdr/attach.go` + MyGo `plugins/terminal` |
| `keys.macos.json` + `src/keys.rs` | `internal/ui/keys.go` |

MyGo's terminal plugin is the biggest shortcut: it wraps libghostty-vt (the
same Ghostty emulator the original uses) and accepts an `io.ReadWriteCloser`
with `Resize`/`ResizePixels`/`UseTheme`, which is exactly what a core attach
stream needs. The rest is hand-drawn with `ui.Row/Column/Text/Icon/Scroll`
against the measurements above.

### Machines

`hc machine ls` gives `name`, `target`, `state` (connected, connecting,
needs_login, unreachable), `why`, `version`. Each machine owns its own
`herdr-app` document, read and written with `on: "<name>"`; ids inside it are
that machine's own (`sp…`, `t3`), and the window qualifies them (`rohan/sp…`,
`rohan/t3`) so two machines never collide. Terminal methods take the
qualified address, so attach/send/resize/focus work unchanged, while
`terminal.run` takes `on`. `events.subscribe` carries remote terminals and
doc changes (with `machine`) in the same stream. The add dialog
(`mods/machine-add`) lists `~/.ssh/config` hosts and a free-form `user@host`;
when a machine needs hc copied, a password or a host key, it runs
`hc machine add` in a terminal inside the dialog and shows it. The sidebar
groups spaces under a machine header only when a second machine is known
(`This computer` first, then machines sorted by name), with fold, state dot,
`+` (new space there) and empty-state `New space there`. Settings → Machines
lists them with Connect / Remove. Git in Changes and the sidebar, plus the
Files panel's folder listing (`sh` script) and file preview (`head -c`), run
through `machine.exec` on that machine; a space whose folder is that machine's
home is named `~` (homes are read once per machine).

Not ported: the layout/mod engine itself (JSON layouts and JavaScript
panels), worktrees, peek and floating blocks, pane/tab drag, `--via` and
temporary machines, session settling, updates and telemetry. See the README
for the list.

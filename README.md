# herdr-mygo

Herdr's desktop UI, rebuilt in Go with [MyGo](https://github.com/egoist/mygo)'s
native UI. The **core is Herdr's own**: [herdr-core](https://github.com/herdrdev/herdr-core)
(`hc`) still runs every terminal, every agent and the spaces/tabs document.
This app is only the window.

![Herdr's main window](docs/screenshot-main.png)

```
hc (herdr-core)                  this app
┌──────────────────────┐        ┌──────────────────────────────┐
│ terminals, PTYs      │ framed │ MyGo native UI (Go, GPU)      │
│ spaces/tabs doc      │◀──────▶│ sidebar · tabs · panes        │
│ agents' status       │  RPC   │ palette · settings · panels   │
└──────────────────────┘        └──────────────────────────────┘
```

## Run

Needs Go 1.27+ and `hc` (herdr-core). For development, install it using the
[upstream instructions](https://github.com/herdrdev/herdr-core#install), or
run a bundled build as described below. The app checks its bundled core first,
then `PATH`, then the upstream install locations (`~/.local/bin` on Unix and
`%LOCALAPPDATA%\Programs\herdr-core` on Windows).

```sh
go run .
```

`herdr-core` keeps running when the window closes, exactly as with the
original app: close the window, your agents keep working.

To build a self-contained app bundle, run MyGo's normal build command:

```sh
go tool mygo build -platform darwin/arm64
```

Use the matching platform on other machines, such as `windows/amd64` or
`linux/amd64`. The `buildCommand` in `mygo.json` automatically stages the
pinned core releases for all five supported targets before MyGo packages the
app, including builds invoked through `scripts/build-all.sh`. A download,
checksum or extraction failure stops the build instead of shipping an app
without its core. No separate staging command is needed.

The staging tool uses the upstream release pinned in
[cmd/prepare-hc/release.json](cmd/prepare-hc/release.json), checks the archive's
SHA-256, and extracts it to `resources/<os>-<arch>/bin/`. MyGo automatically
copies those files into the target app: `Contents/Resources/bin/` in a macOS
bundle, or `bin/` beside the executable on Windows and Linux. Distribute the
whole app directory or package, including `bin/` and the native libraries.
Copying just the Windows `.exe` is insufficient.

The staged files include licenses and notices, Windows' required `conpty/`
helpers, and the signed `Herdr Core.app` on macOS. The app uses that nested
macOS bundle to retain the core's privacy identity. The core is ready offline
at app startup; it is not downloaded or installed into the user's system.
New cores inherit the bundled CLI directory on `PATH`, so their terminals can
also run `hc`. Downloaded resources are ignored by git. To update the bundled
core, update the version, URLs and checksums in `release.json` from the upstream
versioned manifest, then rebuild.

Upstream publishes macOS and Linux builds for arm64 and amd64, and Windows
for amd64 only. Staging `windows/arm64` fails explicitly because there is no
native upstream release. To build all five supported targets:

```sh
go tool mygo build \
  -platform darwin/arm64,darwin/amd64,windows/amd64,linux/arm64,linux/amd64
```

The first run downloads MyGo's `libghostty-vt` (the terminal emulator library
the terminal plugin uses, the same Ghostty VT the original app uses).

To build all five supported targets (macOS and Linux for arm64 and amd64,
Windows for amd64) in one run:

```sh
./scripts/build-all.sh
```

Run with Bash (on Windows, use Git Bash or WSL). The script works from any
working directory and writes each target to `build/<os>-<arch>/`, with macOS
named `darwin`. MyGo includes the required native libraries and creates these
packages:

| Target | Packages | Build host requirements |
|---|---|---|
| macOS (arm64, amd64) | `.app` bundle and `herdr-0.1.0-<arch>.dmg` | DMGs and signing require macOS; other hosts still build the `.app` |
| Windows (amd64) | `.exe` and `herdr-setup-0.1.0-amd64.exe` installer | NSIS (`makensis`); MyGo downloads it automatically on Windows if missing |
| Linux (arm64, amd64) | `.deb`, `.tar.gz`, a separate `install.sh`, and `PKGBUILD` | No extra packaging tools; building an Arch package from `PKGBUILD` requires `makepkg` on Arch Linux |

On macOS or Linux, install NSIS to also create Windows installers (for example,
`brew install nsis` on macOS or `sudo apt install nsis` on Debian/Ubuntu).
Without NSIS on those hosts, MyGo warns and skips Windows installers. It also
warns and skips DMGs on hosts other than macOS. macOS signing is ad hoc by
default; see [macOS distribution and Gatekeeper](#macos-distribution-and-gatekeeper)
for the current download and opening instructions.

These are cross-compiled builds; running each app requires its target OS. The
script builds the UI and packages with the matching core resources included
automatically. Windows arm64 packaging is not supported. Builds require network
access to the pinned releases. `-skip-build-command` explicitly bypasses core
staging; use it only when the required resources have already been prepared
with `go run ./cmd/prepare-hc`. Plain `go run .` and `go tool mygo dev` still
need an installed core or manually staged development bundle.
Additional MyGo build flags can be passed through, for example
`./scripts/build-all.sh -o build/release` or
`./scripts/build-all.sh -skip-dmg -skip-installer` to omit DMG and Windows
installer creation explicitly.

## macOS distribution and Gatekeeper

This project's current release path does not use an Apple Developer Program
account. The macOS app uses an ad hoc signature and is **not notarized by
Apple**. You can publish the complete DMGs produced locally or by CI without
Apple credentials, but downloaders should expect Gatekeeper to block the first
open with a warning such as **“Apple could not verify ‘Herdr’ is free of malware
that may harm your Mac or compromise your privacy.”** The exact wording varies
by macOS version. These packages do not pass the default Gatekeeper checks.

A local `go tool mygo build -platform darwin/arm64` build can run on the build
machine because local builds normally have no `com.apple.quarantine` attribute.
Browsers generally add this attribute to downloads, and it can follow an app
copied from a DMG or extracted from an archive. Both local and CI builds use
ad hoc signing; the local build is not more trusted by Apple. Re-uploading a
local build, changing the archive format, or removing quarantine before
upload does not prevent the recipient's browser from quarantining the download.
A previous manual approval can also affect whether a Mac prompts again.

If you trust the source and choose to run this unnotarized release, follow
Apple's [Open Anyway instructions](https://support.apple.com/en-us/102445):

1. Download the matching `herdr-<version>-arm64.dmg` for Apple silicon or
   `herdr-<version>-amd64.dmg` for an Intel Mac. Open the DMG and drag
   **Herdr** onto **Applications**.
2. In Finder, open **Applications** and double-click **Herdr**. When macOS
   shows the verification warning, dismiss it with **Done** or **OK**,
   depending on the version; keep the app rather than moving it to the Trash.
   You must try opening the app before the next step.
3. Open the **Apple menu → System Settings → Privacy & Security**. Scroll
   down to **Security**, find the message that Herdr was blocked, and click
   **Open Anyway** (Chinese: **仍要打开**). If the button is missing, try
   opening Herdr again, then return to this settings page. On a managed Mac,
   your administrator may restrict these settings.
4. When the warning appears again, click **Open** to confirm. Authenticate
   if macOS asks. This saves an exception for the app, so subsequent launches
   can use the usual double-click. A new build may require approval again.

This is a manual exception on each recipient's Mac; the package remains
unnotarized. These steps apply to the developer-verification warning above.
If macOS instead reports detected malware or a damaged app, stop and obtain
a fresh copy or contact the maintainer. No terminal command or global
Gatekeeper setting change is needed for the Open Anyway procedure.

If the project later enrolls in the Apple Developer Program, it can issue
releases signed with a **Developer ID Application** certificate and private
key, submitted to Apple's notarization service, and shipped with the accepted
tickets stapled to the DMGs. MyGo supports that process, but the release Mac
or CI must first be configured with the actual certificate and notarization
credentials, and the resulting downloaded packages must be verified.
An **Apple Development** certificate alone does not provide this distribution
trust. Only publishing those signed and notarized builds would change the
current Gatekeeper experience; a normal first-open confirmation for a
downloaded app can still appear afterward.

## GitHub Actions packages

[Build packages](.github/workflows/build.yml) runs automatically on pushes to
`master` and can also be run manually from the repository's Actions tab. It
uses three standard GitHub-hosted runners:
macOS builds `darwin/arm64,darwin/amd64`, Windows builds `windows/amd64`, and
Linux builds `linux/arm64,linux/amd64`. Each runs `go tool mygo build -platform`
with those targets, using the Go version in `go.mod` and the normal bundled
core staging hook. The macOS runner creates DMGs, and the Windows runner can
download NSIS automatically to create the Windows installer.

The run uploads three artifacts, `herdr-darwin`, `herdr-windows` and
`herdr-linux`, retained for seven days. They contain only the five native
installers:

| Platform | Release assets |
|---|---|
| macOS | `herdr-<version>-arm64.dmg`, `herdr-<version>-amd64.dmg` |
| Windows | `herdr-setup-<version>-amd64.exe` |
| Linux (Debian/Ubuntu) | `herdr_<debian-version>_arm64.deb`, `herdr_<debian-version>_amd64.deb` |

Each installer includes the app, bundled core, notices and native libraries.
Download the matching installer directly from the release. The workflow fails
if any expected installer is missing or empty. Complete build directories,
portable archives, `install.sh` and Arch `PKGBUILD` recipes are not release
assets; they remain local MyGo build outputs. A `PKGBUILD` is a recipe, not a
built Arch installer. Actions wraps its downloadable artifacts in ZIP files;
GitHub Release assets are the native installer files themselves.

After every build succeeds, a push to `master` publishes these five installers
as GitHub Release assets in a prerelease. The version is
`<base>-dev.<run number>.g<short commit SHA>`, for example
`0.1.0-dev.42.gabcdef1`, and its tag is `v0.1.0-dev.42.gabcdef1`. Every target
uses that same resolved version; MyGo converts Debian prerelease versions to
use `~` before the suffix (for example `0.1.0~dev.42.gabcdef1`) and replaces
hyphens within the suffix with dots. The release points to the exact commit
that was built. Separate workflow runs get separate versions; reruns of the
same run use the same version.

Pushing a `v*` tag also runs the builds and publishes a versioned GitHub Release.
The tag must match `mygo.json`'s version exactly, such as `v0.1.0` for `0.1.0`;
a mismatch fails before the platform builds start. A version with a prerelease
suffix, such as `0.2.0-rc.1`, produces a prerelease. Other branches do not trigger
automatic builds or releases. Manual runs build the same development packages
and only upload artifacts. The release job uses the repository's built-in
`GITHUB_TOKEN` with `contents: write`; no additional release secret is needed.
macOS packages use the current ad hoc signing configuration and are not
notarized. Follow the [Open Anyway procedure](#macos-distribution-and-gatekeeper)
after downloading; no Apple credentials are required for this release path.

Standard hosted runner compute is free for public repositories; artifact and
cache storage have separate allowances. See [GitHub Actions billing](https://docs.github.com/en/billing/concepts/product-billing/github-actions).

## Version management

`mygo.json` is the single source for the app's planned release version. Use
[Semantic Versioning](https://semver.org/): `MAJOR.MINOR.PATCH`, optionally
followed by a prerelease such as `-rc.1`. Store no `v` prefix or `+build` metadata
in this field. Tags add the `v` prefix. The bundled hc version in
`cmd/prepare-hc/release.json` is an independent upstream dependency.

```bash
go run ./cmd/version                 # show the planned version
go run ./cmd/version bump patch      # 0.1.0 -> 0.1.1 (bug fixes)
go run ./cmd/version bump minor      # 0.1.1 -> 0.2.0 (features)
go run ./cmd/version bump major      # 0.2.0 -> 1.0.0 (stable contract / breaking changes)
go run ./cmd/version set 0.2.0-rc.1   # choose an explicit release candidate
```

Bumps reset lower components to zero and remove any prerelease suffix. Commit
`mygo.json` with the change that needs the new version. During `0.x` development,
use minor bumps for substantial or incompatible features and patch bumps for
fixes. After `1.0`, increment major for incompatible changes, minor for compatible
features and patch for fixes. Do not infer release importance from commit text.

Every push to `master` automatically publishes a development prerelease without
editing or committing the base version back to the repository. A prerelease base
is extended with `.dev.<run>.g<sha>` (for example `0.2.0-rc.1.dev.42.gabcdef1`).
For an official release, commit the desired version first, then tag that exact
commit as `v<version>` and push the tag. GitHub generates release notes. After an
official release, bump the base for the next planned release before further
feature work, so development packages sort below that upcoming release. Keep
published version tags fixed; use a new version for changed packages.

MyGo embeds the resolved package version. Settings reads that version from MyGo
instead of keeping another version constant; a plain unpackaged `go run` shows
`dev`. Local MyGo builds use the checked-in version. CI computes one version
before starting the build matrix, applies it to each runner's copy of `mygo.json`,
and uses the same value for the release. `go test ./cmd/version` checks bumps,
validation, tag matching and development versions without publishing anything.

These conventions follow [MyGo's tag-driven release guide](https://mygo.egoist.dev/docs/github-actions)
and [GoReleaser's use of SemVer Git tags](https://goreleaser.com/getting-started/quick-start/).
[Tiny RDM's Wails workflow](https://github.com/tiny-craft/tiny-rdm/blob/main/.github/workflows/release-macos.yaml)
also derives a version from the release tag, writes it into the package config,
and passes it to the app. Herdr keeps its existing MyGo packaging workflow and
adds automatic development releases alongside official tagged releases.

## Languages

The interface follows the system’s preferred languages by default. English,
Chinese (Simplified and Traditional) and Japanese are supported; if none of the
preferred languages is supported, it uses English. Settings → Look → Language
can keep English, 中文 (Simplified Chinese) or 日本語 instead, or return to System.
The choice applies immediately and is saved as `language` in `settings.json`.
Menus, native controls and dialogs use the same language as the interface.
Terminal output, file contents, paths and user-assigned names retain their text.

Messages live in standard ARB/ICU JSON catalogs in `internal/i18n/locales`, with
English as the source and fallback. MyGo generates and registers their Go
accessors, including its own control translations. After editing catalogs, run:

```sh
go tool mygo generate
go tool mygo i18n check
go test ./...
```

Commit `catalog_gen.go` with catalog changes so plain Go builds work too.
`internal/i18n/messages.go` maps the UI’s English source strings to stable catalog
keys; ICU placeholders and plural forms let translations order arguments and
counts independently.

## What is here

| Files | What it does |
|---|---|
| `internal/core` | `hc connect`, the framed JSON-RPC protocol: calls, event streams, terminal attach streams, flow control |
| `internal/herdr` | the world: the `herdr-app` document (spaces → tabs → pane trees), terminal records and events, commands, git, the program table, and the attach connection that makes a core terminal a MyGo terminal |
| `internal/ui` | the window: theme, frame, sidebar, tabs, pane cards, status bar, command palette, picker, new-tab menu, context menus, settings, welcome, Files and Changes panels, toasts |
| `internal/assets` | the original app's icons, fonts (JetBrains Mono, Geist) and logo, under their licenses |

The window follows the shipped `main` layout: a 268 px sidebar with the
spaces list, the agents list and the footer; a 42 px tab bar; pane cards
(radius 10, 1 px border, accent border while focused, 30 px header); a
28 px status bar; Files/Changes as a 368 px side panel. The default colors come from
Herdr's `graphite` theme, resolved with the same formula Herdr uses
(`settings.rs`), and terminals use the same palette, font and padding.
Settings → Look also offers Kero's tinted themes and gradient backgrounds,
with separate choices for light and dark windows, plus searchable Ghostty
themes (including installed files in Ghostty's themes folder). The chrome
and every open terminal follow the chosen palette. Choose “Theme” under
Accent to use the palette's accent, or pick an override.

Settings → Keyboard shortcuts lets you search app actions, record, add or
remove shortcuts, and warns about conflicts. Changes take effect immediately.
The defaults on Linux include `Super+D` to split right and `Super+Shift+D`
to split down, alongside `Ctrl+Shift+O` and `Ctrl+Shift+E`. On macOS,
`Cmd+D` and `Cmd+Shift+D` split right and down. Terminal copy and paste use
separate shortcuts.

Shortcut changes are saved to `~/.config/herdr-mygo/keys.json` (or under
`$XDG_CONFIG_HOME` or `$HERDR_MYGO_HOME`). Without a custom file, Herdr uses
the shipped platform defaults. Terminals receive other keys through Ghostty's
key protocol handling in MyGo's terminal plugin.

## What works

- Spaces, tabs and splits stored in the core's shared document, with labels
  (`herdr.space`, `herdr.tab`) so terminals started outside the app join them
- **SSH machines**: add one from `~/.ssh/config` or by typing how ssh reaches
  it (Settings → Machines); each machine
  keeps its own copy of the document (`doc.get/set … on`), its spaces are
  grouped under its header in the sidebar, and its terminals run, attach,
  split, resize and report agents through the same core, addressed
  `machine/t3`. Remote spaces are first class: new space from the machine's
  `+`, tabs and splits there, a name of `~` from that machine's own home,
  Files listing and previewing that machine's folders (`machine.exec` + `sh`),
  and Changes running git there. A machine that needs herdr-core copied, a
  password or a host key opens `hc machine add` in a terminal inside the
  dialog. Settings →
  Machines lists each machine with its state, and Connect / Remove. SSH
  keepalives and connection reuse are always managed by herdr-core; the app
  has no keepalive switch
- Terminal attach: live output, typing, resize, colors reported to programs,
  selection, copy/paste, links, scrollback (MyGo terminal plugin)
- Find in terminal output and scrollback: ⌘F (Ctrl+Shift+F elsewhere),
  or “Find in terminal” in the palette or pane's right-click menu. Matches
  highlight as you type; Enter / Shift+Enter and the arrow buttons move
  toward older / newer output, with a match count. ⌘G / ⌘⇧G (F3 / Shift+F3)
  also navigate and reopen the last query. Escape closes the search and
  returns typing to the terminal. Peeked sub-agents can be searched too
- Focus and zoom per tab, click to focus, accent focus border
- Space/tab/pane commands: new, close, rename, move, select, next/prev,
  split right/down, close, zoom, focus/swap by direction. Closing a pane,
  tab or space asks for confirmation when it would end a running program;
  idle shells and finished terminals close immediately
- The agents list from terminals that report status (OSC 7501), by priority,
  each agent with its own logo: Herdr's program table's icons (Claude Code,
  Codex, Pi, OpenCode, Grok, Gemini, …), full color where the mark is, and
  DeepSeek's for `dsh-tui`/`dsh`; a row from another machine names it, and
  the workers an agent started (`hc run --bg …` from its terminal) hang under
  its row with the lines that lead to them, and a click on one looks at it
  over the tab: the bar over the terminal keeps it as a pane of the shown
  tab, or closes the look
- Command palette with fuzzy search, group headers and key chips
- Go-to-space-or-tab picker, new-tab menu, right-click menus on spaces,
  tabs and panes, prompt dialogs, toasts
- Settings for Look, Terminal, Machines and Alerts, with
  search (⌘F), theme previews and live controls; welcome and the keys list.
  Appearance follows the desktop, or stays light or dark. Light and dark
  choices persist separately in `light_theme` and `dark_theme`; older
  single-`theme` settings retain their previous appearance fallback.
  Graphite, Ink, Moss and Paper remain available alongside Kero's Petal,
  Mist, Meadow, Dusk, Tide, Apricot, Aurora, Dune, Neutral and Plain.
  Neutral is Kero's paper wash, named separately to keep Herdr's Paper palette.
  Look's font controls list bundled and installed families (monospaced families
  for terminals), with editable sizes from 8 to 40. Interface type updates
  across the window; terminal type updates open panes and peek without
  reconnecting. Font choices, sizes and window zoom persist in `settings.json`
- Alerts follow agent status reports: Done and Needs you send system notifications
  for agents in another space, with optional sound. On macOS, run
  `go tool mygo dev` or the app from `go tool mygo build` for notifications;
  the first alert asks for notification permission. Linux uses the desktop's
  notification service through `notify-send` (install `libnotify-bin` on
  Debian/Ubuntu or `libnotify` on Fedora/Arch), with a logged-in graphical
  session. Linux packages include these dependencies. Windows 10/11 uses Windows
  PowerShell and native WinRT toasts,
  registered as Herdr for the current user; allow Herdr in Settings → System →
  Notifications. Linux and Windows alerts also work with `go run .`.
  Repeated reports and the initial session snapshot stay quiet. Glass and Ping
  are built-in chimes; either event can use a custom mono/stereo WAV file (up to 10 seconds and
  10 MB), previewed from Settings. By agent mutes sound for a program while
  keeping its system notifications. Sound choices and per-agent mutes persist in
  `settings.json`; disabling Play a sound keeps notifications enabled.
- The app keeps its own files, apart from herdr-app's, under
  `~/.config/herdr-mygo`: `settings.json` written as a setting changes, the
  keys, and the screen's view
- `herdr-app do` reaches the window: it answers on
  `$HERDR_APP_HOME/herdr-app.sock` (else `~/.config/herdr-app`), where that
  command looks, one JSON line each way. `tab.new` starts a tab and
  `terminal.keep` gives a terminal that is already running — a worker the
  core started — a pane, so it lands in the window
- Files tree with file preview; Changes with git status, diff and commit
- The sidebar collapses to nothing — ⌘B, or the button in the brand bar —
  easing closed as MyGo's sidebar example does (`Animate` + `ClipX`), and
  opens again at the width it had; the width and whether it is open are
  kept in `settings.json`
- Sidebar drag to resize, spaces and machine groups dragged into order,
  panes resized by dragging the divider between them, window drag on the
  brand and tab bars, traffic lights where Herdr puts them

## What is not here yet

The original app is a general layout/mod engine: layouts are JSON, panels
are JavaScript mods. This port draws Herdr's shipped interface in Go, so:

- No `layouts/*.json` or `mods/*` execution: `terminals` and `win98` layouts,
  and user mods, do not load. Their look is fixed to `main`.
- No worktrees, terminal peek or floating blocks; panes can be split,
  focused and resized by their divider, but not dragged to move; tabs
  cannot be drag-reordered yet.
- Machines are added over ssh only (no `--via` commands or temporary
  machines); Files reads remote folders one level at a time through
  `machine.exec`, so it has no git-ignore dimming and refreshes every few
  seconds rather than on every change.
- No session settling, update checks or telemetry, and only `tab.new` and
  `terminal.keep` answer `herdr-app do`: `ls`, `status`, `shot` and the rest
  of its commands are not here yet. Previews (Files, Changes) are read-only
  except the commit box.
- Option as Alt is hidden while MyGo v0.7.4 lacks runtime and left/right
  Option control. Existing saved values are retained for future support.
- Updates and Privacy are hidden from Settings and its search until an updater
  and telemetry exist. Existing saved values are retained for future support.

## How it is checked

`internal/ui/shot_test.go` renders the window without a window and saves a
2× PNG, which is compared against the original's own `herdr-app shot` at the
same size and scale. The screenshots in `docs/` were made that way.

```sh
SHOT_OUT=/tmp/main.png go test ./internal/ui -run TestShot
SHOT_OVERLAY=palette SHOT_OUT=/tmp/palette.png go test ./internal/ui -run TestShot
SHOT_PANEL=files SHOT_OUT=/tmp/files.png go test ./internal/ui -run TestShot
SHOT_ACTION=pane.split:down SHOT_OUT=/tmp/split.png go test ./internal/ui -run TestShot
```

The Linux and Windows notification command boundaries are covered by the alert
unit tests. On each target desktop, the following opt-in check sends a real
Done and Needs you notification; confirm both appear under Herdr, with literal
Unicode/markup text and no system sound:

```sh
ALERTS_NATIVE_NOTIFICATIONS=1 go test ./internal/ui -run '^TestNativeDesktopNotifications$' -v
```

On Windows PowerShell, set `$env:ALERTS_NATIVE_NOTIFICATIONS='1'` first, then run
that `go test` command. This requires a logged-in desktop with notification
banners enabled; passing the command confirms service acceptance, while the
visible banners must be checked on that desktop.

## License

Herdr MyGo is licensed under the [MIT License](LICENSE). Third-party assets and
dependencies retain their own licenses and notices.

## Research

[docs/herdr-ui-spec.md](docs/herdr-ui-spec.md) is the study of the original
app this port is built from: the model, the layout metrics, the theme, every
panel and overlay, keys and commands, and where each piece lives in Go.

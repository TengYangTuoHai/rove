package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

type alertState struct {
	customize bool
	sounds    map[string]*audio.Sound
	// The native boundaries are replaceable in control tests.
	play   func(string) error
	pick   func() ([]string, error)
	notify func(mygo.NotificationOptions) error
	// pending is written by notification callbacks and consumed on the UI thread.
	pending []herdr.Agent
}

func validAlertSound(name string) bool {
	return name == "Glass" || name == "Ping" || filepath.IsAbs(name)
}

func alertSoundLabel(name string) string { return filepath.Base(name) }

func alertAgentKey(program herdr.ProgramInfo) string {
	if program.Key == "agent" {
		return strings.ToLower(program.Name)
	}
	return program.Key
}

// agentAlerts runs on the UI thread, so the event stream never reads settings
// while a control edits them. Taking the queue also consumes disabled alerts.
func (a *App) agentAlerts() {
	for _, ag := range a.S.TakeAgentAlerts() {
		a.showAgentAlert(ag)
	}
}

func (a *App) showAgentAlert(ag herdr.Agent) {
	sound, title := a.settings.DoneSound, a.tr("Done")
	switch ag.Status {
	case "done":
		if !a.settings.AlertDone {
			return
		}
	case "blocked":
		if !a.settings.AlertNeeds {
			return
		}
		sound, title = a.settings.NeedsSound, a.tr("Needs you")
	default:
		return
	}
	name := ag.Title
	if name == "" {
		name = ag.Program.Name
	}
	if name == "" {
		name = ag.Address
	}
	body := ag.SpaceName
	if body == "" {
		body = ag.Program.Name
	}
	notify := a.alerts.notify
	if notify == nil {
		notify = func(opts mygo.NotificationOptions) error {
			return sendSystemAgentNotification(opts, func() { a.activateAgentAlert(ag) })
		}
	}
	// Authorization can wait for the user. Keep it off the UI thread and
	// leave sound playback governed by the existing app preferences.
	go func() {
		err := notify(mygo.NotificationOptions{Title: title + " · " + name, Body: body, Silent: true})
		if err != nil {
			message := a.tr(err.Error())
			if errors.Is(err, mygo.ErrNotificationsDenied) {
				message = a.tr("Allow Rove notifications in system notification settings.")
			}
			a.toast(a.tr("Cannot show system notification"), message, "error")
		}
	}()
	if a.settings.AlertSound && !a.settings.MutedAgents[alertAgentKey(ag.Program)] {
		a.previewAlertSound(sound)
	}
}

// activateAgentAlert queues navigation for the UI thread and presents the window.
func (a *App) activateAgentAlert(ag herdr.Agent) {
	a.mu.Lock()
	a.alerts.pending = append(a.alerts.pending, ag)
	a.mu.Unlock()
	if a.Win != nil {
		a.Win.Show()
	}
	a.Repaint()
}

func (a *App) openAgentAlerts() {
	a.mu.Lock()
	pending := a.alerts.pending
	a.alerts.pending = nil
	a.mu.Unlock()
	for _, ag := range pending {
		if ag.Address != "" && a.S.FocusTerminal(ag.Address) {
			a.CloseOverlay()
		} else if ag.Space != "" && a.spaceByID(ag.Space) != nil {
			a.S.SelectSpace(0, ag.Space)
			a.CloseOverlay()
		}
	}
}

func sendSystemAgentNotification(opts mygo.NotificationOptions, onClick func()) error {
	if runtime.GOOS == "linux" {
		// notify-send waits until the notification is clicked or dismissed.
		return sendDesktopNotificationAction(context.Background(), "linux", opts, onClick)
	}
	return sendSystemNotification(opts)
}

func sendSystemNotification(opts mygo.NotificationOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if runtime.GOOS != "darwin" {
		return sendDesktopNotification(ctx, runtime.GOOS, opts)
	}
	if !mygo.NotificationsSupported() {
		return fmt.Errorf("system notifications require a packaged app; run go tool mygo dev or go tool mygo build")
	}
	return mygo.NewNotification(opts).ShowContext(ctx)
}

func (a *App) previewAlertSound(name string) {
	play := a.alerts.play
	if play == nil {
		play = a.playAlertSound
	}
	if err := play(name); err != nil {
		a.toast(a.tr("Cannot play sound"), err.Error(), "error")
	}
}

func (a *App) playAlertSound(name string) error {
	if a.alerts.sounds == nil {
		a.alerts.sounds = map[string]*audio.Sound{}
	}
	sound := a.alerts.sounds[name]
	if sound == nil {
		var err error
		sound, err = loadAlertSound(name)
		if err != nil {
			return err
		}
		a.alerts.sounds[name] = sound
	}
	sound.Play()
	return audio.Err()
}

// loadAlertSound uses two short, built-in chimes on every desktop. Custom
// effects are WAV files of at most ten seconds, decoded before accepting them.
func loadAlertSound(name string) (*audio.Sound, error) {
	if name == "Glass" || name == "Ping" {
		const rate = 48000
		samples := make([]float32, rate/2)
		for i := range samples {
			t := float64(i) / rate
			frequency := 880.0
			if name == "Glass" {
				frequency = 1320
			}
			tone := math.Sin(2 * math.Pi * frequency * t)
			if name == "Glass" {
				tone = 0.7*tone + 0.3*math.Sin(2*math.Pi*frequency*2.76*t)
			}
			envelope := min(t/0.005, 1) * math.Exp(-12*t) * min((0.5-t)/0.02, 1)
			samples[i] = float32(0.25 * envelope * tone)
		}
		return audio.NewSound(rate, 1, samples), nil
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > 10<<20 {
		return nil, fmt.Errorf("choose a WAV file smaller than 10 MB")
	}
	decoder, _, err := audio.NewDecoder(io.NewSectionReader(file, 0, stat.Size()))
	if err != nil {
		return nil, fmt.Errorf("choose a valid WAV sound: %w", err)
	}
	if decoder.SampleRate() <= 0 || decoder.SampleRate() > 192000 ||
		decoder.Channels() < 1 || decoder.Channels() > 2 || decoder.Len() <= 0 ||
		decoder.Len() > int64(decoder.SampleRate())*10 {
		return nil, fmt.Errorf("choose a mono or stereo WAV sound up to 10 seconds long")
	}
	return audio.ReadSound(decoder)
}

func (a *App) chooseAlertSound(label string, selected *string) {
	pick := a.alerts.pick
	if pick == nil {
		pick = func() ([]string, error) {
			return mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent: a.Win, Title: a.tr("Choose %s", strings.ToLower(label)),
				Filters: []mygo.FileFilter{{Name: a.tr("WAV sounds"), Extensions: []string{"wav"}}},
			})
		}
	}
	paths, err := pick()
	if err != nil {
		a.toast(a.tr("Cannot choose sound"), err.Error(), "error")
		return
	}
	if len(paths) == 0 {
		return
	}
	path, err := filepath.Abs(paths[0])
	if err == nil {
		var sound *audio.Sound
		sound, err = loadAlertSound(path)
		if err == nil {
			if a.alerts.sounds == nil {
				a.alerts.sounds = map[string]*audio.Sound{}
			}
			a.alerts.sounds[path] = sound
			*selected = path
			a.Repaint()
			return
		}
	}
	a.toast(a.tr("Cannot choose sound"), err.Error(), "error")
}

// settingsSound is a preview, the built-in choices and a custom WAV picker.
func (a *App) settingsSound(c *ui.Context, label string, selected *string, enabled bool) {
	t := a.T
	play := ui.Box(c).Size(26, 26).Radius(13).Border(1, t.Input).Center().Label(a.tr("Preview %s", label))
	if enabled {
		play.Cursor(ui.CursorPointer)
		if play.Clicked() {
			a.previewAlertSound(*selected)
		}
	}
	play.Children(func() { a.icon(c, "play", 10, t.Muted) })
	if !enabled {
		ui.Text(c, alertSoundLabel(*selected)).FontSize(a.fontSize(12)).TextColor(t.Muted).Width(110).Ellipsis("…")
		a.settingsButton(c, a.tr("Choose…"), false, true, nil)
		return
	}
	sel := ui.SelectBase(c.Key(label), selected)
	sel.Trigger.Row().Width(110).Height(26).PaddingX(8).Gap(4).Radius(6).
		Border(1, t.Input).AlignItems(ui.Center).Label(a.tr("%s selection", label)).Children(func() {
		ui.Text(c, alertSoundLabel(*selected)).FontSize(a.fontSize(12)).TextColor(t.Muted).Grow(1).Ellipsis("…")
		a.icon(c, "chevron-down", 10, t.Dim)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Width(180).Padding(4).Radius(8).Border(1, t.Border).Background(t.Popover)
		options := []string{"Glass", "Ping"}
		if filepath.IsAbs(*selected) {
			options = append(options, *selected)
		}
		for _, name := range options {
			item := sel.Item(name).FillWidth().Padding(6, 8).Radius(4).Label(alertSoundLabel(name))
			if item.Highlighted() {
				item.Background(t.Hover)
			}
			item.Children(func() {
				ui.Text(c, alertSoundLabel(name)).FontSize(a.fontSize(12)).TextColor(t.Foreground).Ellipsis("…")
			})
		}
	})
	ui.Box(c).Label(a.tr("Choose %s", label)).Children(func() {
		a.settingsButton(c, a.tr("Choose…"), false, true, func() { a.chooseAlertSound(label, selected) })
	})
}

func (a *App) agentSoundSummary() string {
	muted := 0
	for _, off := range a.settings.MutedAgents {
		if off {
			muted++
		}
	}
	if muted == 0 {
		return a.tr("All agents on")
	}
	return a.tr("%d muted", muted)
}

func (a *App) alertPrograms() []herdr.ProgramInfo {
	programs := herdr.AgentPrograms()
	known := map[string]bool{}
	for _, p := range programs {
		known[alertAgentKey(p)] = true
	}
	add := func(key, name string) {
		if key != "" && !known[key] {
			programs = append(programs, herdr.ProgramInfo{Key: key, Name: name, Icon: "agent", Tint: true, Agent: true})
			known[key] = true
		}
	}
	if a.S != nil {
		for _, ag := range a.S.Agents() {
			add(alertAgentKey(ag.Program), ag.Program.Name)
		}
	}
	for key := range a.settings.MutedAgents {
		add(key, key)
	}
	sort.Slice(programs, func(i, j int) bool { return programs[i].Name < programs[j].Name })
	return programs
}

func (a *App) settingsAgentSounds(c *ui.Context) {
	for i, program := range a.alertPrograms() {
		if i > 0 {
			a.settingsDivider(c)
		}
		key := alertAgentKey(program)
		a.settingsRow(c.Key(key), program.Name, "", func() {
			on := !a.settings.MutedAgents[key]
			ui.Box(c).Label(a.tr("%s sound", program.Name)).Children(func() { a.settingsToggle(c, &on) })
			if on {
				delete(a.settings.MutedAgents, key)
			} else {
				if a.settings.MutedAgents == nil {
					a.settings.MutedAgents = map[string]bool{}
				}
				a.settings.MutedAgents[key] = true
			}
		})
	}
}

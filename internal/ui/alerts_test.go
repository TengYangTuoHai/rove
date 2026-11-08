package ui

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	myui "github.com/egoist/mygo/ui"

	"rove/internal/herdr"
)

func testAlertWAV(t *testing.T) string {
	t.Helper()
	const rate = 8000
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+rate/5))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	for _, value := range []uint16{1, 1} {
		binary.Write(&b, binary.LittleEndian, value)
	}
	for _, value := range []uint32{rate, rate * 2} {
		binary.Write(&b, binary.LittleEndian, value)
	}
	for _, value := range []uint16{2, 16} {
		binary.Write(&b, binary.LittleEndian, value)
	}
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(rate/5))
	for i := 0; i < rate/10; i++ {
		binary.Write(&b, binary.LittleEndian, int16(5000*math.Sin(2*math.Pi*440*float64(i)/rate)))
	}
	path := filepath.Join(t.TempDir(), "custom.wav")
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAlertPreferencesSurviveLaunch(t *testing.T) {
	t.Setenv("ROVE_HOME", t.TempDir())
	first := NewApp(nil, ThemeByName("graphite"))
	first.LoadSettings()
	path := testAlertWAV(t)
	first.settings.DoneSound, first.settings.NeedsSound = path, "Glass"
	first.settings.MutedAgents["pi"] = true
	first.settings.AlertDone, first.settings.AlertNeeds, first.settings.AlertSound = false, false, false
	first.saveSettings()
	second := NewApp(nil, ThemeByName("graphite"))
	second.LoadSettings()
	if second.settings.DoneSound != path || second.settings.NeedsSound != "Glass" || !second.settings.MutedAgents["pi"] ||
		second.settings.AlertDone || second.settings.AlertNeeds || second.settings.AlertSound {
		t.Fatalf("alert settings did not survive: %+v", second.settings)
	}
	legacy := readSettings([]byte(`{"alert_sound":false}`), defaultSettings())
	if legacy.DoneSound != "Glass" || legacy.NeedsSound != "Ping" || legacy.AlertSound || legacy.MutedAgents == nil {
		t.Fatalf("legacy settings lost defaults: %+v", legacy)
	}
	invalid := readSettings([]byte(`{"done_sound":"unknown","needs_sound":"","muted_agents":null}`), defaultSettings())
	if invalid.DoneSound != "Glass" || invalid.NeedsSound != "Ping" || invalid.MutedAgents == nil {
		t.Fatalf("invalid settings were not sanitized: %+v", invalid)
	}
}

func TestAgentAlertSettingsGateNotificationAndSound(t *testing.T) {
	for _, tc := range []struct {
		name, status              string
		done, needs, sound, muted bool
		wantNotes, wantSounds     int
		wantSound                 string
	}{
		{"done", "done", true, true, true, false, 1, 1, "Glass"},
		{"needs", "blocked", true, true, true, false, 1, 1, "Ping"},
		{"done off", "done", false, true, true, false, 0, 0, ""},
		{"needs off", "blocked", true, false, true, false, 0, 0, ""},
		{"sound off", "done", true, true, false, false, 1, 0, ""},
		{"agent muted", "blocked", true, true, true, true, 1, 0, ""},
		{"working", "working", true, true, true, false, 0, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				app := NewApp(nil, ThemeByName("graphite"))
				app.settings.AlertDone, app.settings.AlertNeeds, app.settings.AlertSound = tc.done, tc.needs, tc.sound
				app.settings.MutedAgents["pi"] = tc.muted
				var sounds []string
				var notifications []mygo.NotificationOptions
				app.alerts.notify = func(opts mygo.NotificationOptions) error {
					notifications = append(notifications, opts)
					return nil
				}
				app.alerts.play = func(name string) error { sounds = append(sounds, name); return nil }
				app.showAgentAlert(herdr.Agent{Address: "t1", Title: "Working session", SpaceName: "Project", Status: tc.status,
					Program: herdr.ProgramInfo{Key: "pi", Name: "Pi"}})
				synctest.Wait()
				if len(notifications) != tc.wantNotes || len(sounds) != tc.wantSounds || len(toastSnapshot(app)) != 0 {
					t.Fatalf("notifications = %+v, sounds = %v, toasts = %+v", notifications, sounds, toastSnapshot(app))
				}
				if len(notifications) > 0 {
					title := "Done"
					if tc.status == "blocked" {
						title = "Needs you"
					}
					if got := notifications[0]; got.Title != title+" · Working session" || got.Body != "Project" || !got.Silent {
						t.Fatalf("notification content or system sound = %+v", got)
					}
				}
				if len(sounds) > 0 && sounds[0] != tc.wantSound {
					t.Fatalf("sound = %q, want %q", sounds[0], tc.wantSound)
				}
			})
		})
	}
	synctest.Test(t, func(t *testing.T) {
		app := NewApp(nil, ThemeByName("graphite"))
		notified := false
		app.alerts.notify = func(mygo.NotificationOptions) error { notified = true; return nil }
		app.alerts.play = func(string) error { return errors.New("missing custom sound") }
		app.showAgentAlert(herdr.Agent{Status: "done", Title: "Agent"})
		synctest.Wait()
		if notes := toastSnapshot(app); !notified || len(notes) != 1 || notes[0].Title != "Cannot play sound" {
			t.Fatalf("playback failure hid the notification or its error: notified = %v, notes = %+v", notified, notes)
		}
	})
}

func TestAlertSoundControlsPreviewSelectChooseAndSave(t *testing.T) {
	RegisterFonts()
	app := NewApp(nil, ThemeByName("graphite"))
	app.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	var played []string
	app.alerts.play = func(name string) error { played = append(played, name); return nil }
	path := testAlertWAV(t)
	picks := 0
	app.alerts.pick = func() ([]string, error) { picks++; return []string{path}, nil }
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Padding(20).Gap(20).Children(func() {
			myui.Row(c).Gap(10).Children(func() { app.settingsSound(c, "Done sound", &app.settings.DoneSound, app.settings.AlertSound) })
			myui.Row(c).Gap(10).Children(func() { app.settingsSound(c, "Needs you sound", &app.settings.NeedsSound, app.settings.AlertSound) })
		})
		app.saveSettings()
	}, 700, 300)
	tt.Frame()
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	click("Preview Done sound")
	click("Preview Needs you sound")
	if strings.Join(played, ",") != "Glass,Ping" {
		t.Fatalf("previews = %v", played)
	}
	click("Done sound selection")
	tt.Key(0, myui.KeyEnd)
	tt.Key(0, myui.KeyEnter)
	tt.Frame()
	if app.settings.DoneSound != "Ping" {
		t.Fatalf("built-in selection did not change: %q", app.settings.DoneSound)
	}
	click("Choose Done sound")
	click("Choose Needs you sound")
	if picks != 2 || app.settings.DoneSound != path || app.settings.NeedsSound != path {
		t.Fatalf("file controls: %d picks, %q/%q", picks, app.settings.DoneSound, app.settings.NeedsSound)
	}
	click("Preview Done sound")
	if played[len(played)-1] != path {
		t.Fatal("preview ignored the selected custom sound")
	}
	data, err := os.ReadFile(app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	saved := readSettings(data, defaultSettings())
	if saved.DoneSound != path || saved.NeedsSound != path {
		t.Fatalf("controls did not save: %+v", saved)
	}
	// Disabled controls neither preview nor launch the picker.
	app.settings.AlertSound = false
	tt.Frame()
	before := len(played)
	click("Preview Done sound")
	if len(played) != before || picks != 2 {
		t.Fatal("disabled sound control performed an action")
	}
}

func TestChooseAlertSoundCancelAndErrorsKeepSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		paths     []string
		err       error
		wantError bool
	}{
		{"cancel", nil, nil, false},
		{"dialog error", nil, errors.New("picker unavailable"), true},
		{"missing file", []string{filepath.Join(t.TempDir(), "missing.wav")}, nil, true},
		{"invalid file", []string{filepath.Join(t.TempDir(), "invalid.wav")}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "invalid file" {
				os.WriteFile(tc.paths[0], []byte("not audio"), 0o600)
			}
			app := NewApp(nil, ThemeByName("graphite"))
			app.alerts.pick = func() ([]string, error) { return tc.paths, tc.err }
			app.chooseAlertSound("Done sound", &app.settings.DoneSound)
			if app.settings.DoneSound != "Glass" {
				t.Fatalf("bad choice replaced selection: %q", app.settings.DoneSound)
			}
			if got := len(toastSnapshot(app)) > 0; got != tc.wantError {
				t.Fatalf("error toast = %v", got)
			}
		})
	}
}

func TestCustomizeAgentSoundsPersistsAndKeepsOtherAgentsOn(t *testing.T) {
	RegisterFonts()
	app := NewApp(nil, ThemeByName("graphite"))
	app.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	tt := myui.NewTester(func(c *myui.Context) {
		myui.Column(c).Width(800).Children(func() { app.settingsAlerts(c) })
		app.saveSettings()
	}, 850, 2100)
	tt.Frame()
	if err := tt.Click("Customize  ›"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.alerts.customize {
		t.Fatal("Customize did not open")
	}
	if err := tt.Click("Codex sound"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.settings.MutedAgents["codex"] || app.settings.MutedAgents["pi"] || app.agentSoundSummary() != "1 muted" {
		t.Fatalf("per-agent setting = %+v (%s)", app.settings.MutedAgents, app.agentSoundSummary())
	}
	data, err := os.ReadFile(app.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved := readSettings(data, defaultSettings()); !saved.MutedAgents["codex"] {
		t.Fatal("mute was not saved")
	}
	if err := tt.Click("Codex sound"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.agentSoundSummary() != "All agents on" {
		t.Fatal("unmute did not restore the summary")
	}
	if err := tt.Click("Hide  ‹"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.alerts.customize {
		t.Fatal("Hide did not close")
	}
}

func TestAgentNotificationDoesNotBlockSettings(t *testing.T) {
	RegisterFonts()
	app := NewApp(&herdr.Session{}, ThemeByName("graphite"))
	app.settings.Section, app.overlay = "alerts", "settings"
	app.settings.AlertSound = false
	authorized := make(chan struct{})
	delivered := make(chan mygo.NotificationOptions, 1)
	app.alerts.notify = func(opts mygo.NotificationOptions) error {
		<-authorized
		delivered <- opts
		return nil
	}
	app.showAgentAlert(herdr.Agent{Status: "done", Title: strings.Repeat("Long session title ", 8), SpaceName: "Background project"})
	tt := myui.NewTester(app.View, 1280, 820)
	tt.Frame()
	if err := tt.Click("Done alerts"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	close(authorized)
	if app.settings.AlertDone || len(toastSnapshot(app)) != 0 || app.overlay != "settings" {
		t.Fatalf("settings blocked or agent toast shown: notes = %v, overlay = %q", toastSnapshot(app), app.overlay)
	}
	select {
	case notification := <-delivered:
		if notification.Body != "Background project" {
			t.Fatalf("notification content = %+v", notification)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("notification not delivered after authorization")
	}
}

func TestAgentNotificationFailureIsActionable(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		body string
	}{
		{"denied", mygo.ErrNotificationsDenied, "Allow Rove notifications in system notification settings."},
		{"delivery failed", errors.New("notification service unavailable"), "notification service unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				app := NewApp(nil, ThemeByName("graphite"))
				app.settings.AlertSound = false
				app.alerts.notify = func(mygo.NotificationOptions) error { return tc.err }
				app.showAgentAlert(herdr.Agent{Status: "blocked", Title: "Agent"})
				synctest.Wait()
				if notes := toastSnapshot(app); len(notes) != 1 || notes[0].Title != "Cannot show system notification" || notes[0].Body != tc.body {
					t.Fatalf("delivery failure = %+v", notes)
				}
			})
		})
	}
}

func TestAlertSoundDecodesAndNativePlayback(t *testing.T) {
	for _, name := range []string{"Glass", "Ping", testAlertWAV(t)} {
		sound, err := loadAlertSound(name)
		if err != nil || sound.Duration() <= 0 || sound.Duration() > time.Second {
			t.Fatalf("sound %s: %v, %v", name, sound, err)
		}
		if os.Getenv("ALERTS_PLAY_AUDIO") != "" {
			player := sound.Play()
			time.Sleep(sound.Duration() + 100*time.Millisecond)
			if err := audio.Err(); err != nil {
				t.Fatal(err)
			}
			player.Close()
		}
	}
}

func TestAlertsScreenshots(t *testing.T) {
	dir := os.Getenv("ALERTS_SHOT_DIR")
	if dir == "" {
		t.Skip("set ALERTS_SHOT_DIR")
	}
	RegisterFonts()
	for _, tc := range []struct {
		name, theme    string
		off, customize bool
	}{
		{"alerts", "graphite", false, false},
		{"sound-off", "paper", true, false},
		{"by-agent", "graphite", false, true},
	} {
		app := NewApp(&herdr.Session{}, ThemeByName(tc.theme))
		app.settings.Section, app.overlay = "alerts", "settings"
		app.settings.AlertSound = !tc.off
		app.alerts.customize = tc.customize
		app.settings.MutedAgents["codex"] = tc.customize
		tt := myui.NewTester(app.View, 1280, 820)
		tt.SetScale(2)
		tt.Frame()
		path := filepath.Join(dir, tc.name+".png")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		t.Log(path)
	}
}

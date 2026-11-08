package ui

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/egoist/mygo"
)

func TestLinuxNotificationTextAndSound(t *testing.T) {
	for _, silent := range []bool{false, true} {
		opts := mygo.NotificationOptions{Title: "--Done · 项目 $(touch /tmp/no)", Body: `<b>space</b> & "project"`, Silent: silent}
		cmd, err := desktopNotificationCommand(context.Background(), "linux", opts)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"notify-send", "--app-name=Rove", "--icon=rove", "--hint=string:desktop-entry:rove"}
		if silent {
			want = append(want, "--hint=boolean:suppress-sound:true")
		}
		want = append(want, "--", opts.Title, "&lt;b&gt;space&lt;/b&gt; &amp; &#34;project&#34;")
		if !reflect.DeepEqual(cmd.Args, want) {
			t.Fatalf("notification arguments = %q, want %q", cmd.Args, want)
		}
	}
}

func TestWindowsNotificationTextIsData(t *testing.T) {
	for _, silent := range []bool{false, true} {
		opts := mygo.NotificationOptions{Title: "Needs you · 项目 🚀", Body: "'\"</text>\n$(exit 42) ` & <project>", Silent: silent}
		cmd, err := desktopNotificationCommand(context.Background(), "windows", opts)
		if err != nil {
			t.Fatal(err)
		}
		wantArgs := []string{"powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand"}
		if !reflect.DeepEqual(cmd.Args[:len(cmd.Args)-1], wantArgs) {
			t.Fatalf("PowerShell arguments = %q", cmd.Args)
		}
		encoded, err := base64.StdEncoding.DecodeString(cmd.Args[len(cmd.Args)-1])
		if err != nil {
			t.Fatal(err)
		}
		units := make([]uint16, len(encoded)/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(encoded[i*2:])
		}
		if got := string(utf16.Decode(units)); got != windowsNotificationScript || strings.Contains(got, opts.Body) {
			t.Fatal("encoded command changed or contains interpolated notification text")
		}
		var payload struct {
			Title, Body string
			Silent      bool
		}
		data, err := io.ReadAll(cmd.Stdin)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Title != opts.Title || payload.Body != opts.Body || payload.Silent != silent {
			t.Fatalf("notification input lost text or sound preference: %+v", payload)
		}
	}
}

func TestDesktopNotificationDeliveryErrorsAndCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake notify-send uses a POSIX shell")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if err := sendDesktopNotification(context.Background(), "linux", mygo.NotificationOptions{}); err == nil || !strings.Contains(err.Error(), "install notify-send") {
		t.Fatalf("missing notify-send error = %v", err)
	}
	fake := filepath.Join(dir, "notify-send")
	for _, tc := range []struct {
		name, script, want string
	}{
		{"delivered", "exit 0", ""},
		{"no session bus", "echo 'Cannot connect to session bus' >&2\nexit 1", "Cannot connect to session bus"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(fake, []byte("#!/bin/sh\n"+tc.script+"\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			err := sendDesktopNotification(context.Background(), "linux", mygo.NotificationOptions{Title: "Done", Silent: true})
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("delivery error = %v, want %q", err, tc.want)
			}
		})
	}
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec /bin/sleep 10\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := sendDesktopNotification(ctx, "linux", mygo.NotificationOptions{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("notification deadline error = %v", err)
	}
	if err := sendDesktopNotification(context.Background(), "plan9", mygo.NotificationOptions{}); err == nil || !strings.Contains(err.Error(), "not supported on plan9") {
		t.Fatalf("unsupported platform error = %v", err)
	}
}

// This opt-in check uses the real host notification service, rather than a
// replacement command. Run on a desktop with notification banners enabled.
func TestNativeDesktopNotifications(t *testing.T) {
	if os.Getenv("ALERTS_NATIVE_NOTIFICATIONS") == "" {
		t.Skip("set ALERTS_NATIVE_NOTIFICATIONS=1 on a Linux or Windows desktop")
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("native notification check requires Linux or Windows")
	}
	for _, title := range []string{"Done · Rove notification check", "Needs you · Rove notification check"} {
		err := sendSystemNotification(mygo.NotificationOptions{Title: title, Body: "项目 <test> & another space", Silent: true})
		if err != nil {
			t.Fatal(err)
		}
	}
}

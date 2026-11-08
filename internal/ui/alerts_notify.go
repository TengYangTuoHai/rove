package ui

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html"
	"os/exec"
	"strings"
	"unicode/utf16"

	"github.com/egoist/mygo"
)

// sendDesktopNotification uses the notification service of the local desktop.
// The caller runs it off the UI thread and supplies a delivery deadline.
func sendDesktopNotification(ctx context.Context, platform string, opts mygo.NotificationOptions) error {
	cmd, err := desktopNotificationCommand(ctx, platform, opts)
	if err != nil {
		return err
	}
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("system notification: %w", ctx.Err())
	}
	if err != nil {
		if platform == "linux" && cmd.Err != nil {
			return fmt.Errorf("install notify-send (libnotify-bin on Debian/Ubuntu or libnotify on Fedora/Arch) to show system notifications: %w", err)
		}
		if message := strings.TrimSpace(string(output)); message != "" {
			return fmt.Errorf("system notification: %s: %w", message, err)
		}
		return fmt.Errorf("system notification: %w", err)
	}
	return nil
}

func desktopNotificationCommand(ctx context.Context, platform string, opts mygo.NotificationOptions) (*exec.Cmd, error) {
	switch platform {
	case "linux":
		args := []string{"--app-name=Rove", "--icon=rove", "--hint=string:desktop-entry:rove"}
		if opts.Silent {
			args = append(args, "--hint=boolean:suppress-sound:true")
		}
		// Only the body supports markup in the freedesktop notification spec.
		// Separate options from agent-supplied text, including leading dashes.
		args = append(args, "--", opts.Title, html.EscapeString(opts.Body))
		return exec.CommandContext(ctx, "notify-send", args...), nil
	case "windows":
		payload, err := json.Marshal(struct {
			Title  string
			Body   string
			Silent bool
		}{opts.Title, opts.Body, opts.Silent})
		if err != nil {
			return nil, err
		}
		// EncodedCommand expects UTF-16LE. Notification text stays on stdin;
		// it is never interpolated into PowerShell source or XML.
		units := utf16.Encode([]rune(windowsNotificationScript))
		encoded := make([]byte, len(units)*2)
		for i, unit := range units {
			binary.LittleEndian.PutUint16(encoded[i*2:], unit)
		}
		cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
		cmd.Stdin = strings.NewReader(string(payload))
		hideNotificationWindow(cmd)
		return cmd, nil
	default:
		return nil, fmt.Errorf("system notifications are not supported on %s", platform)
	}
}

const windowsNotificationScript = `
$ErrorActionPreference = 'Stop'
try {
    $inputReader = [System.IO.StreamReader]::new([Console]::OpenStandardInput(), [System.Text.Encoding]::UTF8)
    $data = $inputReader.ReadToEnd() | ConvertFrom-Json
    $null = [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]
    $null = [Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime]
    $null = [Windows.UI.Notifications.NotificationSetting, Windows.UI.Notifications, ContentType = WindowsRuntime]
    $null = [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime]

    $appID = 'dev.rove.app'
    $key = 'HKCU:\Software\Classes\AppUserModelId\' + $appID
    if (-not (Test-Path -LiteralPath $key)) {
        $null = New-Item -Path $key -Force
    }
    $null = New-ItemProperty -Path $key -Name 'DisplayName' -Value 'Rove' -PropertyType String -Force
    # A stable stub CLSID keeps unpackaged toasts in Notification Center.
    # These alerts do not provide activation actions.
    $null = New-ItemProperty -Path $key -Name 'CustomActivator' -Value '{7E53EBD8-52B3-4C66-A159-F81DC2E771B4}' -PropertyType String -Force

    $xml = New-Object Windows.Data.Xml.Dom.XmlDocument
    $xml.LoadXml('<toast><visual><binding template="ToastGeneric"><text/><text/></binding></visual></toast>')
    $text = $xml.GetElementsByTagName('text')
    $null = $text.Item(0).AppendChild($xml.CreateTextNode([string]$data.Title))
    $null = $text.Item(1).AppendChild($xml.CreateTextNode([string]$data.Body))
    if ($data.Silent) {
        $audio = $xml.CreateElement('audio')
        $audio.SetAttribute('silent', 'true')
        $null = $xml.DocumentElement.AppendChild($audio)
    }
    $notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($appID)
    if ($notifier.Setting -ne [Windows.UI.Notifications.NotificationSetting]::Enabled) {
        throw 'Allow Rove notifications in Windows Settings > System > Notifications.'
    }
    $toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
    $notifier.Show($toast)
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
`

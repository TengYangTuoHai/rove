package herdr

import (
	"os"
	"testing"
)

func TestTerminalEnvAdvertisesImagesAndKeepsUserChoice(t *testing.T) {
	for _, tc := range []struct {
		name, value, want string
		unset             bool
	}{
		{name: "automatic", unset: true, want: "kitty"},
		{name: "empty", value: "", want: "kitty"},
		{name: "disabled", value: "none", want: "none"},
		{name: "explicit kitty", value: "kitty", want: "kitty"},
		{name: "explicit sixel", value: "sixel", want: "sixel"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DSH_TUI_IMAGE_PROTOCOL", tc.value)
			if tc.unset {
				if err := os.Unsetenv("DSH_TUI_IMAGE_PROTOCOL"); err != nil {
					t.Fatal(err)
				}
			}
			env := splitEnv()
			if got := env["DSH_TUI_IMAGE_PROTOCOL"]; got != tc.want {
				t.Fatalf("image protocol = %q, want %q", got, tc.want)
			}
			if env["TERM"] != "xterm-256color" || env["COLORTERM"] != "truecolor" {
				t.Fatalf("terminal capabilities lost: %v", env)
			}
		})
	}
}

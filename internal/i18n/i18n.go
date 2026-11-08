// Package i18n adapts the application's source messages to MyGo's ARB catalogs.
package i18n

import (
	"fmt"
	"strconv"
	"strings"

	native "github.com/egoist/mygo/i18n"
	"rove/internal/i18n/locales"
)

// Resolve chooses a supported language from ordered BCP 47 or POSIX preferences.
// MyGo provides the desktop's preference list and reports changes at runtime.
func Resolve(preferences []string) string {
	normalized := make([]string, 0, len(preferences))
	for _, locale := range preferences {
		if i := strings.IndexAny(locale, ".@"); i >= 0 {
			locale = locale[:i]
		}
		if tag := native.Normalize(strings.ReplaceAll(locale, "_", "-")); tag != "" {
			normalized = append(normalized, tag)
		}
	}
	if matched := native.Match(normalized, []string{"en", "zh", "zh-Hant", "ja"}); matched != "" {
		return matched
	}
	return "en"
}

// Localizer is immutable and safe to share with notification goroutines.
type Localizer struct{ language string }

func New(locale string) *Localizer    { return &Localizer{Resolve([]string{locale})} }
func (l *Localizer) Language() string { return l.language }

// Text uses English source text as the call-site ID. Catalogs use stable named
// keys and ICU placeholders; argument order is independent in each language.
func (l *Localizer) Text(source string, args ...any) string {
	key, ok := messageIDs[source]
	if !ok {
		return source
	}
	var values map[string]any
	if len(args) > 0 {
		values = make(map[string]any, len(args))
		for i, arg := range args {
			values["arg"+strconv.Itoa(i+1)] = arg
		}
		// Quoted search queries stay quoted while translations reorder placeholders.
		at := 0
		for i := 0; i < len(source)-1; i++ {
			if source[i] == '%' {
				i++
				if source[i] == '%' {
					continue
				}
				if source[i] == 'q' && at < len(args) {
					values["arg"+strconv.Itoa(at+1)] = fmt.Sprintf("%q", args[at])
				}
				at++
			}
		}
	}
	return locales.Catalog.Format(l.language, key, values)
}

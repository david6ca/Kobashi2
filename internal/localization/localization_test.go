package localization

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestLocaleNegotiationAndOpaqueArguments(t *testing.T) {
	for _, c := range []struct {
		selection   string
		preferences []string
		want        string
	}{
		{"auto", []string{"xx", "de-AT", "zh-TW"}, "de"},
		{"auto", []string{"zh-Hans-HK"}, "zh-Hans"},
		{"auto", []string{"zh-Hant-CN"}, "zh-Hant"},
		{"auto", []string{"zh-HK"}, "zh-Hant"},
		{"auto", []string{"pt-PT"}, "pt-BR"},
		{"ja", []string{"en-US"}, "ja"},
		{"auto", []string{"ar-EG"}, "ar"},
		{"auto", []string{"xx"}, "en"},
	} {
		if got := Resolve(c.selection, c.preferences); got != c.want {
			t.Errorf("Resolve(%q,%v)=%q; want %q", c.selection, c.preferences, got, c.want)
		}
	}
	if got := Format("de", "{0} tokens", "private {1} Settings"); got != "private {1} Settings Tokens" {
		t.Fatal("argument was translated or interpolated twice", got)
	}
}

func TestAllThirteenCatalogsAndPrivacyAreComplete(t *testing.T) {
	var value struct {
		Languages []Language
		Messages  map[string]map[string]string
	}
	if err := json.Unmarshal(Data, &value); err != nil {
		t.Fatal(err)
	}
	expected := []string{"en", "zh-Hans", "zh-Hant", "ja", "ko", "de", "fr", "pt-BR", "es", "ar", "hi", "ru", "vi"}
	var got []string
	for _, language := range value.Languages {
		got = append(got, language.Code)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatal("locale scope changed", got)
	}
	placeholder := regexp.MustCompile(`\{[0-9]+\}`)
	for source, translations := range value.Messages {
		want := placeholder.FindAllString(translations["en"], -1)
		for _, locale := range expected {
			translation := translations[locale]
			if strings.TrimSpace(translation) == "" {
				t.Errorf("%s missing %s", locale, source)
			}
			actual := placeholder.FindAllString(translation, -1)
			if !reflect.DeepEqual(actual, want) {
				t.Errorf("%s changed placeholders for %s", locale, source)
			}
		}
	}
	for _, locale := range expected {
		privacy := Privacy(locale)
		if !strings.Contains(privacy, "support@pomegram.com") || !strings.Contains(privacy, "GitHub") || strings.Contains(privacy, "privacy.") {
			t.Errorf("incomplete privacy statement %s", locale)
		}
	}
}

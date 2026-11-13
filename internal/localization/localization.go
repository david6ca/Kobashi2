// Package localization owns the product's shared native UI and privacy copy.
package localization

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
)

//go:embed catalog.json
var Data []byte

type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

var catalog = func() struct {
	Languages []Language                   `json:"languages"`
	Messages  map[string]map[string]string `json:"messages"`
} {
	var value struct {
		Languages []Language                   `json:"languages"`
		Messages  map[string]map[string]string `json:"messages"`
	}
	if err := json.Unmarshal(Data, &value); err != nil {
		panic(err)
	}
	return value
}()

func Languages() []Language { return append([]Language(nil), catalog.Languages...) }

func Valid(selection string) bool {
	if selection == "auto" {
		return true
	}
	for _, language := range catalog.Languages {
		if selection == language.Code {
			return true
		}
	}
	return false
}

func Resolve(selection string, preferences []string) string {
	if selection != "auto" && Valid(selection) {
		return selection
	}
	for _, preference := range preferences {
		parts := strings.Split(strings.ToLower(strings.ReplaceAll(preference, "_", "-")), "-")
		if parts[0] == "zh" {
			for _, part := range parts[1:] {
				if part == "hant" {
					return "zh-Hant"
				}
				if part == "hans" {
					return "zh-Hans"
				}
			}
			for _, part := range parts[1:] {
				if part == "tw" || part == "hk" || part == "mo" {
					return "zh-Hant"
				}
			}
			return "zh-Hans"
		}
		if parts[0] == "pt" {
			return "pt-BR"
		}
		for _, language := range catalog.Languages {
			if parts[0] == language.Code {
				return language.Code
			}
		}
	}
	return "en"
}

// Text translates authored copy only. Callers must not pass account/model names,
// user input, URLs or content returned by an inference provider.
func Text(locale, source string) string {
	if translated := catalog.Messages[source][locale]; translated != "" {
		return translated
	}
	if english := catalog.Messages[source]["en"]; english != "" {
		return english
	}
	return source
}

func Format(locale, source string, arguments ...string) string {
	pairs := make([]string, 0, len(arguments)*2)
	for i, argument := range arguments {
		pairs = append(pairs, "{"+strconv.Itoa(i)+"}", argument)
	}
	return strings.NewReplacer(pairs...).Replace(Text(locale, source))
}

func Privacy(locale string) string {
	sections := make([]string, 6)
	for i := range sections {
		sections[i] = Text(locale, "privacy."+strconv.Itoa(i))
	}
	return strings.Join(sections, "\n\n")
}

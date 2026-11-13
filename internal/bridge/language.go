package bridge

import (
	"encoding/json"
	"fmt"

	"github.com/david6ca/Kobashi2/internal/localization"
)

func (a *App) loadLanguage() {
	a.language = "auto"
	data, err := readOptional(a.path("language.json"))
	if err == nil && data != nil {
		var selection string
		if err = json.Unmarshal(data, &selection); err == nil {
			if localization.Valid(selection) {
				a.language = selection
			} else {
				err = fmt.Errorf("invalid language: %q", selection)
			}
		}
	}
	if err != nil {
		a.report(err)
	}
}

func (a *App) saveLanguage(selection string) (object, error) {
	if !localization.Valid(selection) {
		return nil, fmt.Errorf("invalid language: %q", selection)
	}
	if err := writeJSON(a.path("language.json"), selection); err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.language = selection
	a.mu.Unlock()
	return object{"language": selection}, nil
}

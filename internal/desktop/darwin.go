//go:build darwin

package desktop

import (
	"errors"
	"fmt"
	"os"

	"github.com/david6ca/Kobashi2/internal/bridge"
)

func FocusExisting() bool      { return false }
func ShowError(message string) { fmt.Fprintln(os.Stderr, message) }
func Run(app *bridge.App, hidden bool) error {
	return errors.New("launch Kobashi2.app for the Cocoa interface, or use --no-open for a headless bridge")
}

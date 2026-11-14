package bridge

import (
	"errors"
	"os/exec"
)

func localHardware() (int64, int64, error) {
	return 0, 0, errors.New("Local models require an Apple Silicon Mac with macOS 15 or later")
}
func localFreeDisk(string) (int64, error) { return 0, errors.New("Local models are available on Mac") }
func configureLocalCommand(c *exec.Cmd)   {}

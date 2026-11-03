package main

import (
	"bytes"
	"io"
	"log"
	"strings"
	"testing"
)

type unavailableConsole struct{}

func (unavailableConsole) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestLoggingSurvivesUnavailableConsole(t *testing.T) {
	output, flags := log.Writer(), log.Flags()
	t.Cleanup(func() {
		log.SetOutput(output)
		log.SetFlags(flags)
	})
	var file bytes.Buffer
	configureLogging(&file, unavailableConsole{})
	log.Print("first diagnostic")
	log.Print("second diagnostic")
	if !strings.Contains(file.String(), "first diagnostic") || !strings.Contains(file.String(), "second diagnostic") {
		t.Fatal("console failure prevented file logging", file.String())
	}
}

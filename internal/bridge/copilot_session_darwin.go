package bridge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func copilotConnectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}

func copilotQuery(path, query string) ([]byte, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/sqlite3", "-readonly", "-noheader", "-cmd", ".timeout 2000", path, query)
	data, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("read the Copilot app selection database: %w", err)
	}
	return data, nil
}

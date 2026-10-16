package bridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

type copilotRouting struct {
	instance string
	explicit bool
	known    map[string]copilotSelection
	pending  map[string]bool
	handled  map[string]bool
	retryAt  map[string]time.Time
}

func (a *App) localCopilotSessions() ([]copilotSelection, error) {
	if a.copilotLocalSessions != nil {
		return a.copilotLocalSessions()
	}
	data, err := copilotQuery(filepath.Join(a.options.Home, ".copilot", "data.db"), copilotLocalSessionsSQL)
	if err != nil {
		return nil, fmt.Errorf("read local Copilot sessions for automatic connection: %w", err)
	}
	return decodeCopilotSessions(data)
}

func sameCopilotChoice(a, b copilotSelection) bool {
	return a.Model == b.Model && a.ProviderID == b.ProviderID &&
		a.ReasoningEffort == b.ReasoningEffort && a.ContextTier == b.ContextTier
}

func (a *App) syncCopilot() error {
	if state := a.snapshot(); !state.active() || !state.Copilot {
		a.copilotRouting = copilotRouting{}
		return nil
	}
	port, owner, err := copilotRunValue(filepath.Join(a.options.Home, ".copilot", "run", "ws.port"))
	if err != nil {
		a.copilotRouting.instance = ""
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect Copilot for automatic connection: %w", err)
	}
	sessions, sessionsErr := a.localCopilotSessions()
	selected, selectionErr := a.selectedCopilotSession()
	routing := &a.copilotRouting
	if routing.known == nil && sessionsErr == nil {
		var saved map[string]copilotSelection
		if !routing.explicit {
			saved, err = a.copilotRestoreSelections()
			if err != nil {
				return err
			}
		}
		routing.known = map[string]copilotSelection{}
		routing.pending = map[string]bool{}
		if routing.handled == nil {
			routing.handled = map[string]bool{}
		}
		for id := range saved {
			routing.handled[id] = true
		}
		for _, session := range sessions {
			routing.known[session.ID] = session
			if session.ID != selected.ID && session.customProvider() {
				routing.handled[session.ID] = true
			}
		}
	}
	if routing.instance != port+"/"+owner {
		if _, err := a.configureCopilot(copilotSelection{}, nil, nil); err != nil {
			return err
		}
		if routing.instance != port+"/"+owner {
			return nil
		}
	}
	if sessionsErr != nil {
		return sessionsErr
	}
	if selectionErr != nil {
		return selectionErr
	}
	current := map[string]copilotSelection{}
	for _, session := range sessions {
		current[session.ID] = session
		if session.ProviderID == copilotProviderID {
			routing.handled[session.ID] = true
		}
		if original, known := routing.known[session.ID]; known {
			if !sameCopilotChoice(original, session) && original.Model != "" {
				routing.handled[session.ID] = true
			} else if original.Model == "" && session.Model != "" {
				routing.known[session.ID] = session
				if session.customProvider() {
					routing.handled[session.ID] = true
				}
			}
		} else {
			routing.known[session.ID] = session
			if !session.customProvider() {
				routing.pending[session.ID] = true
			} else {
				routing.handled[session.ID] = true
			}
		}
	}
	if session, ok := current[selected.ID]; ok && !routing.handled[session.ID] {
		routing.pending[session.ID] = true
	}
	ids := make([]string, 0, len(routing.pending))
	for id := range routing.pending {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var setupErr error
	for _, id := range ids {
		session, exists := current[id]
		if !exists || routing.handled[id] {
			delete(routing.pending, id)
			delete(routing.retryAt, id)
			continue
		}
		if session.ready() != nil || session.Model == "" || time.Now().Before(routing.retryAt[id]) {
			continue
		}
		read := func() (copilotSelection, error) {
			sessions, err := a.localCopilotSessions()
			if err != nil {
				return copilotSelection{}, err
			}
			for _, current := range sessions {
				if current.ID == id {
					return current, nil
				}
			}
			return copilotSelection{}, errors.New("Copilot session was archived during automatic connection")
		}
		if _, err := a.configureCopilot(session, nil, read); err != nil {
			if routing.retryAt == nil {
				routing.retryAt = map[string]time.Time{}
			}
			routing.retryAt[id] = time.Now().Add(5 * time.Second)
			setupErr = errors.Join(setupErr, err)
		}
		if routing.handled[id] {
			delete(routing.pending, id)
			delete(routing.retryAt, id)
		}
	}
	return setupErr
}

func (a *App) startCopilotWatcher() {
	a.copilotWatchOnce.Do(func() {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			var lastError string
			var retryAt time.Time
			for {
				select {
				case <-a.ctx.Done():
					return
				case <-ticker.C:
					if time.Now().Before(retryAt) || !a.actions.TryLock() {
						continue
					}
					if a.ctx.Err() != nil {
						a.actions.Unlock()
						return
					}
					err := a.syncCopilot()
					a.actions.Unlock()
					if err != nil {
						if a.ctx.Err() != nil {
							return
						}
						if err.Error() != lastError {
							a.report(err)
						}
						lastError = err.Error()
						retryAt = time.Now().Add(5 * time.Second)
					}
				}
			}
		}()
	})
}

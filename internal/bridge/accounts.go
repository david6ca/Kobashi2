package bridge

import (
	"context"
	"errors"
	"strings"
)

type account struct {
	Token    string `json:"github_token"`
	Username string `json:"username"`
	Method   string `json:"authMethod"`
}

func upsertAccount(accounts []account, value account) []account {
	next := append([]account(nil), accounts...)
	for i, existing := range next {
		if strings.EqualFold(existing.Username, value.Username) {
			next[i] = value
			return next
		}
	}
	return append(next, value)
}

func savedAccounts(s session) []account {
	accounts := append([]account(nil), s.Accounts...)
	if s.active() {
		accounts = upsertAccount(accounts, account{Token: s.Token, Username: s.Username, Method: s.Method})
	}
	return accounts
}

func publicAccounts(s session) []object {
	result := []object{}
	for _, account := range savedAccounts(s) {
		result = append(result, object{"username": account.Username,
			"selected": s.active() && strings.EqualFold(account.Username, s.Username)})
	}
	return result
}

func (a *App) switchAccount(ctx context.Context, username string) (object, error) {
	if username == "" {
		return nil, errors.New("account username is required")
	}
	s := a.snapshot()
	for _, account := range savedAccounts(s) {
		if strings.EqualFold(account.Username, username) {
			if s.active() && strings.EqualFold(s.Username, username) {
				return object{"ok": true}, nil
			}
			return a.validateToken(ctx, account.Token, account.Method)
		}
	}
	return nil, errors.New("saved account not found")
}

func (a *App) deleteAccount(username string) error {
	if username == "" {
		return errors.New("account username is required")
	}
	s := a.snapshot()
	next := []account{}
	found := false
	for _, account := range savedAccounts(s) {
		if strings.EqualFold(account.Username, username) {
			found = true
		} else {
			next = append(next, account)
		}
	}
	if !found {
		return errors.New("saved account not found")
	}
	if s.active() && strings.EqualFold(s.Username, username) {
		// Restore dependent clients before removing the credential they use.
		return a.disconnectAccounts(next)
	}
	s.Accounts = next
	if err := writeJSON(a.path("session.json"), s); err != nil {
		return err
	}
	a.mu.Lock()
	a.state = s
	a.mu.Unlock()
	return nil
}

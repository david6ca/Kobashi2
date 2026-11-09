package bridge

import (
	"context"
	"errors"
	"strings"
	"time"
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
	if s.Token != "" {
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
	for _, id := range s.LocalModels {
		if model, ok := findLocalModel(id); ok {
			result = append(result, object{"username": "local:" + id, "displayName": model.Name, "provider": "local", "selected": s.LocalModel == id})
		}
	}
	return result
}

// Account actions are serialized by actions. Saving a verified account does not
// require idle inference routes; changing the selected identity still does.
func (a *App) saveVerifiedAccount(token, username, method string, value object) (object, error) {
	previous := a.snapshot()
	if !previous.active() {
		return nil, errors.New("A request is running. Try again when it finishes")
	}
	next := previous
	next.Accounts = upsertAccount(savedAccounts(previous), account{Token: token, Username: username, Method: method})
	sameAccount := previous.LocalModel == "" && strings.EqualFold(previous.Username, username)
	if sameAccount {
		// Reauthorization refreshes credentials for the same identity. Requests
		// already sent keep their own headers; retries may use the new token.
		next.Token, next.Username, next.Method = token, username, method
	}
	if err := writeJSON(a.path("session.json"), next); err != nil {
		return nil, err
	}
	if sameAccount {
		a.setGitHubSession(next, value)
	} else {
		a.mu.Lock()
		a.state.Accounts = next.Accounts
		a.mu.Unlock()
	}
	return object{"ok": true, "username": username}, nil
}

func (a *App) setGitHubSession(next session, value object) {
	a.models.pause()
	a.mu.Lock()
	a.state = next
	a.bearer = str(value["token"])
	a.expiry = number(value["expires_at"])
	a.rawModels = nil
	a.rawAt = time.Time{}
	a.usage = nil
	a.usageAt = time.Time{}
	a.lastEffort = nil
	a.accountGeneration++
	a.mu.Unlock()
	a.models.setAccount(next.Token)
}

func (a *App) switchAccount(ctx context.Context, username string) (object, error) {
	if username == "" {
		return nil, errors.New("account username is required")
	}
	if strings.HasPrefix(username, "local:") {
		return a.local.switchModel(ctx, strings.TrimPrefix(username, "local:"))
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
	if strings.HasPrefix(username, "local:") {
		return a.local.deleteModel(strings.TrimPrefix(username, "local:"))
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

//go:build !appstore

package bridge

func (l *localManager) bundledPython() string            { return "" }
func (l *localManager) storePythonEnvironment() []string { return nil }
func (l *localManager) modelDownloadScript() string      { return l.path("support.py") }

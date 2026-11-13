//go:build appstore && darwin

package bridge

func (l *localManager) bundledPython() string {
	return bundledRuntimePath("python", "bin", "python3.12")
}
func (l *localManager) storePythonEnvironment() []string {
	return []string{"PYTHONHOME=" + bundledRuntimePath("python"), "PYTHONPATH=" + bundledRuntimePath("site-packages"), "PYTHONDONTWRITEBYTECODE=1"}
}
func (l *localManager) modelDownloadScript() string { return bundledRuntimePath("support.py") }

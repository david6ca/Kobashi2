package bridge

import (
	"errors"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func localHardware() (int64, int64, error) {
	if runtime.GOARCH != "arm64" {
		return 0, 0, errors.New("Local models require an Apple Silicon Mac")
	}
	version, err := exec.Command("/usr/bin/sw_vers", "-productVersion").Output()
	if err != nil {
		return 0, 0, err
	}
	major, _ := strconv.Atoi(strings.Split(strings.TrimSpace(string(version)), ".")[0])
	if major < 15 {
		return 0, 0, errors.New("Local models require macOS 15 or later")
	}
	raw, err := exec.Command("/usr/sbin/sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, 0, err
	}
	total, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return 0, 0, err
	}
	vm, err := exec.Command("/usr/bin/vm_stat").Output()
	if err != nil {
		return total, 0, err
	}
	page := int64(16384)
	text := string(vm)
	if _, after, ok := strings.Cut(text, "page size of "); ok {
		n := strings.Fields(after)
		if len(n) > 0 {
			page, _ = strconv.ParseInt(n[0], 10, 64)
		}
	}
	available := int64(0)
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && contains([]string{"Pages free", "Pages inactive", "Pages speculative", "Pages purgeable"}, key) {
			n, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(value), "."), 10, 64)
			available += n * page
		}
	}
	return total, min(total, available), nil
}
func localFreeDisk(path string) (int64, error) {
	var st syscall.Statfs_t
	err := syscall.Statfs(path, &st)
	return int64(st.Bavail) * int64(st.Bsize), err
}
func configureLocalCommand(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
	c.WaitDelay = 3 * time.Second
}

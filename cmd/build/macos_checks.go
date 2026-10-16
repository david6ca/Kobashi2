package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func checkMac(smoke bool) error {
	if !smoke {
		for _, test := range []string{"macos-artwork.swift", "macos-startup.swift", "macos-toggles.swift", "macos-appearance.swift"} {
			if err := checkMacHarness(test); err != nil {
				return err
			}
		}
	}
	test := "macos-lifecycle.swift"
	if smoke {
		test = "macos-smoke.swift"
	}
	return checkMacHarness(test)
}

func checkMacHarness(test string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("Cocoa lifecycle checks require a macOS graphical login session")
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	temp, err := localPath(os.TempDir(), root)
	if err != nil {
		return err
	}
	workspace, err := os.MkdirTemp(temp, "kobashi-cocoa-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)
	source, err := os.ReadFile(filepath.Join(root, "macos", "main.swift"))
	if err != nil {
		return err
	}
	source = []byte(strings.ReplaceAll(string(source), "\r\n", "\n"))
	if display := os.Getenv("KOBASHI_TEST_SCREEN_ID"); display != "" {
		if _, err := strconv.ParseUint(display, 10, 32); err != nil {
			return errors.New("KOBASHI_TEST_SCREEN_ID must be a numeric macOS display ID")
		}
		position := "        window.center()"
		if strings.Count(string(source), position) != 1 {
			return errors.New("Cocoa test window position not found")
		}
		placement := `
        guard let screen = NSScreen.screens.first(where: {
            ($0.deviceDescription[NSDeviceDescriptionKey("NSScreenNumber")] as? NSNumber)?.uint32Value == ` + display + `
        }) else {
            fputs("The requested test display is not connected.\n", stderr)
            exit(1)
        }
        window.collectionBehavior.insert(.moveToActiveSpace)
        window.setFrameOrigin(NSPoint(x: screen.visibleFrame.midX - window.frame.width / 2,
                                      y: screen.visibleFrame.midY - window.frame.height / 2))`
		source = []byte(strings.Replace(string(source), position, placement, 1))
	}
	entry := "\nlet app = NSApplication.shared\n"
	if strings.Count(string(source), entry) != 1 {
		return errors.New("Cocoa application entry point not found")
	}
	harness, err := os.ReadFile(filepath.Join(root, "test", test))
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(workspace, "main.swift"), append([]byte(strings.SplitN(string(source), entry, 2)[0]+"\n"), harness...), 0600); err != nil {
		return err
	}
	app := filepath.Join(workspace, "Kobashi2 Lifecycle Test.app", "Contents")
	if err = os.MkdirAll(filepath.Join(app, "MacOS"), 0700); err != nil {
		return err
	}
	for _, name := range []string{"github.png", "claude.png", "codex.png"} {
		if err = copyFile(filepath.Join(root, "assets", name), filepath.Join(app, "Resources", name)); err != nil {
			return err
		}
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleExecutable</key><string>lifecycle-check</string><key>CFBundleName</key><string>Kobashi2 Lifecycle Test</string><key>CFBundleIdentifier</key><string>com.xjin6.kobashi.lifecycle-test</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>`
	if err = os.WriteFile(filepath.Join(app, "Info.plist"), []byte(plist), 0600); err != nil {
		return err
	}
	executable := filepath.Join(app, "MacOS", "lifecycle-check")
	if err = run(workspace, os.Environ(), "xcrun", "--toolchain", "XcodeDefault", "swiftc", "-framework", "Cocoa", filepath.Join(workspace, "main.swift"), "-o", executable); err != nil {
		return err
	}
	return run(workspace, os.Environ(), executable)
}

func check(_ condition: @autoclosure () -> Bool, _ message: String) {
    guard condition() else {
        fputs("FAIL: \(message)\n", stderr)
        exit(1)
    }
    print("PASS: \(message)")
}

func contrast(_ foreground: NSColor, _ background: NSColor) -> Double {
    func luminance(_ color: NSColor) -> Double {
        let rgb = color.usingColorSpace(.sRGB)!
        let channels = [rgb.redComponent, rgb.greenComponent, rgb.blueComponent].map {
            let value = Double($0)
            return value <= 0.04045 ? value / 12.92 : pow((value + 0.055) / 1.055, 2.4)
        }
        return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722
    }
    let a = luminance(foreground), b = luminance(background)
    return (max(a, b) + 0.05) / (min(a, b) + 0.05)
}

func captureWindow(_ window: NSWindow, _ name: String) {
    guard let directory = ProcessInfo.processInfo.environment["KOBASHI_UI_CAPTURE_DIR"] else { return }
    guard let content = window.contentView,
          let bitmap = content.bitmapImageRepForCachingDisplay(in: content.bounds) else {
        check(false, "The native window supports bitmap capture")
        return
    }
    content.cacheDisplay(in: content.bounds, to: bitmap)
    do {
        try FileManager.default.createDirectory(atPath: directory, withIntermediateDirectories: true)
        guard let data = bitmap.representation(using: .png, properties: [:]) else {
            check(false, "The native capture encodes as PNG")
            return
        }
        try data.write(to: URL(fileURLWithPath: directory).appendingPathComponent(name + ".png"))
    } catch { check(false, "The native capture saves: \(error)") }
}

let application = NSApplication.shared
application.setActivationPolicy(.regular)
let subject = AppDelegate()
let service = Process()
service.executableURL = URL(fileURLWithPath: "/bin/cat")
let serviceInput = Pipe()
service.standardInput = serviceInput
service.standardOutput = FileHandle.nullDevice
try service.run()
subject.serverProcess = service
subject.serverInput = serviceInput
defer {
    serviceInput.fileHandleForWriting.closeFile()
    service.waitUntilExit()
}
subject.setUpStatusItem()
subject.setUpMainMenu()
check(subject.statusItem?.button?.title == "" &&
      subject.statusItem?.button?.image?.isTemplate == true &&
      subject.statusItem?.button?.image?.size == NSSize(width: 22, height: 18),
      "The menu bar uses the connection mark instead of a letter, with system-controlled contrast")
check(subject.createWindow(), "The native window loads its existing brand artwork")
check(subject.window.title == "ModelBridge" &&
      subject.controls["title"] == nil &&
      subject.statusItem?.button?.toolTip == "ModelBridge" &&
      subject.statusItem?.button?.accessibilityLabel() == "ModelBridge",
      "Window, header, and menu-bar accessibility use ModelBridge")
check(application.mainMenu?.items.first?.submenu?.items.first?.title == "About ModelBridge" &&
      application.mainMenu?.items.first?.submenu?.items.last?.title == "Quit ModelBridge" &&
      subject.statusItem?.menu?.items.last?.title == "Quit ModelBridge",
      "Application and menu-bar menus use the renamed app")
check(subject.controls["startupHelp"] == nil && subject.controls["trayHelp"] == nil &&
      subject.controls["theme"] == nil,
      "Settings descriptions and the header theme toggle are removed")
check((subject.controls["startingText"] as? NSTextField)?.stringValue == "Starting..." &&
      subject.controls.filter { !$0.value.isHidden }.map { $0.key } == ["startingText"],
      "Startup waits with plain text instead of offering sign-in")
subject.currentView = "idle"
subject.render()
check(subject.window.contentView?.bounds.size == windowSize,
      "The compact window has a fixed 400 by 390 content area")
check(!subject.window.styleMask.contains(.resizable), "The window cannot be resized")
check(subject.statusItem?.menu?.items.first?.title == "Show ModelBridge",
      "The menu bar provides a way to restore the window")
subject.window.performClose(nil)
check(!subject.window.isVisible, "The red close action hides the window")
check(application.activationPolicy() == .accessory, "Closing to tray removes the Dock entry")
check(!subject.applicationShouldTerminateAfterLastWindowClosed(application),
      "Closing the last window does not terminate the application")

let originalWindow = subject.window
let originalContent = subject.window.contentView
let lifecycle: NSApplicationDelegate = subject
_ = lifecycle.applicationShouldHandleReopen?(application, hasVisibleWindows: false)
check(subject.window.isVisible, "Dock reopening restores a closed window")
check(subject.window === originalWindow && subject.window.contentView === originalContent,
      "Dock reopening preserves the same native window and controls")

for _ in 0..<3 {
    subject.window.performClose(nil)
    _ = lifecycle.applicationShouldHandleReopen?(application, hasVisibleWindows: false)
}
check(subject.window === originalWindow && subject.window.contentView === originalContent,
      "Repeated close and reopen does not create new UI instances")
check(subject.serverProcess === service && service.isRunning,
      "The same child process survives closing and reopening")
subject.window.miniaturize(nil)
_ = lifecycle.applicationShouldHandleReopen?(application, hasVisibleWindows: true)
check(!subject.window.isMiniaturized && subject.window.isVisible,
      "Dock reopening restores a minimized window")
application.hide(nil)
_ = lifecycle.applicationShouldHandleReopen?(application, hasVisibleWindows: false)
check(subject.window.isVisible, "Dock reopening orders a hidden app's window front")
check(application.activationPolicy() == .regular, "The application remains Dock-eligible")
subject.setUpMainMenu()
let quitItem = application.mainMenu?.items.first?.submenu?.items.first { $0.keyEquivalent == "q" }
check(quitItem?.action == #selector(NSApplication.terminate(_:)),
      "Cmd-Q still targets application termination")
subject.closeToTray = false
check(subject.windowShouldClose(subject.window),
      "Disabling close to tray allows the native close action")
check(subject.applicationShouldTerminateAfterLastWindowClosed(application),
      "Disabling close to tray terminates after the last window closes")
subject.applicationWillTerminate(Notification(name: NSApplication.willTerminateNotification))
service.waitUntilExit()
check(!service.isRunning && service.terminationStatus == 0,
      "Explicit application termination closes its own child input gracefully")
check(subject.controls["token"] == nil,
      "Manual token sign-in is absent from the native UI")
/*
check(subject.controls["remoteToken"] is NSSecureTextField &&
      subject.controls["serverToken"] is NSSecureTextField,
      "Client and Server access tokens use protected native fields")
subject.clientView()
check(subject.controls["remoteURL"]?.isHidden == false &&
      subject.controls["remoteConnect"]?.isHidden == false &&
      subject.controls["networkDone"]?.isHidden == false,
      "Client connection controls are reachable without GitHub login")
check((subject.controls["remoteURL"] as? NSTextField)?.accessibilityLabel() == "Server URL",
      "The Server URL has an accessible name")
subject.networkDone()
subject.serverView()
check(subject.controls["listen"]?.isHidden == false &&
      subject.controls["serverToggle"]?.isHidden == false,
      "Server sharing controls are reachable from Settings")
subject.networkDone()
check(subject.controls["clientView"]?.isHidden == false &&
      subject.controls["remoteURL"]?.isHidden == true,
      "Back restores Settings and hides Client input")
*/
for key in ["clientView", "serverView", "remoteURL", "remoteToken", "remoteConnect",
            "discover", "listen", "serverToken", "serverToggle", "serverCopy", "networkDone"] {
    check(subject.controls[key] == nil, "Network control \(key) is absent")
}
check(subject.controls["startup"] is NSButton &&
      (subject.controls["startup"] as? NSButton)?.action == #selector(AppDelegate.saveStartup),
      "Startup uses a native checkbox")
check((subject.controls["startup"] as? NSButton)?.state == .off &&
      (subject.controls["tray"] as? NSButton)?.state == .on,
      "Login startup is opt-in and closing to tray is enabled by default")
check((subject.controls["startup"] as? NSButton)?.accessibilityRole() == .checkBox,
      "Custom styling preserves native checkbox accessibility")
let closeButton = subject.window.standardWindowButton(.closeButton)!
let settingsButton = subject.controls["settings"]!
check(subject.window.toolbarStyle == .unified &&
      settingsButton.convert(NSPoint(x: settingsButton.bounds.midX, y: settingsButton.bounds.midY), to: nil).y ==
      closeButton.convert(NSPoint(x: closeButton.bounds.midX, y: closeButton.bounds.midY), to: nil).y,
      "Settings shares the padded native unified window-control row")
check(subject.controls["subtitle"] == nil, "The route description is absent")
check(NSImage(systemSymbolName: "gearshape", accessibilityDescription: nil) != nil,
      "The standard Settings gear is available")
for (key, title, action) in [
    ("connect", "Not Connected", #selector(AppDelegate.addAccount)),
    ("disconnect", "Connected", #selector(AppDelegate.disconnect))
] {
    check((subject.controls[key] as? NSButton)?.title == title &&
          (subject.controls[key] as? NSButton)?.action == action &&
          (subject.controls[key] as? NSButton)?.accessibilityLabel() == title &&
          (subject.controls[key] as? NSButton)?.accessibilityRole() == .button,
          "The \(key) status retains its native accessible name and action")
}
check(subject.controls["codex"]?.frame == NSRect(x: 18, y: 109, width: 364, height: 44) &&
      subject.controls["claude"]?.frame == NSRect(x: 18, y: 153, width: 364, height: 44) &&
      subject.controls["copilot"]?.frame == NSRect(x: 18, y: 197, width: 364, height: 44),
      "The client rows show Codex, Claude Code, GitHub Copilot in one padded group")
check(subject.controls["startup"]?.frame == NSRect(x: 32, y: 118, width: 336, height: 30) &&
      subject.controls["tray"]?.frame == NSRect(x: 32, y: 166, width: 336, height: 30) &&
      subject.controls["appearance"]?.frame == NSRect(x: 188, y: 214, width: 172, height: 30),
      "Settings uses three evenly spaced rows without descriptions")
let appearanceSelector = subject.controls["appearance"] as! NSPopUpButton
check(appearanceSelector.itemTitles == ["Auto", "Light", "Dark"] &&
      appearanceSelector.selectedItem?.title == "Auto" &&
      appearanceSelector.accessibilityLabel() == "Appearance" &&
      appearanceSelector.accessibilityHelp() == "Auto follows your system appearance.",
      "Appearance exposes explicit native Auto, Light and Dark choices")
check(subject.controls["done"]?.frame == NSRect(x: 32, y: 302, width: 336, height: 38) &&
      subject.controls["quit"] == nil,
      "Settings keeps the full-width Done button without a quit link")
check((subject.controls["done"] as? NSButton)?.title == "Done" &&
      (subject.controls["done"] as? NSButton)?.action == #selector(AppDelegate.done),
      "Done retains native button semantics")
for connected in [false, true] {
    subject.status["connected"] = connected
    subject.currentView = "settings"
    subject.done()
    check(subject.currentView == (connected ? "connected" : "idle"),
          "Done returns to the appropriate main screen")
    let frame = subject.window.frame
    let settingsButton = subject.controls["settings"] as! NSButton
    settingsButton.performClick(nil)
    check(subject.currentView == "settings" && !settingsButton.isHidden &&
          settingsButton.isEnabled && settingsButton.accessibilityLabel() == "Settings" &&
          subject.window.firstResponder === subject.controls["startup"],
          "The Settings icon opens Settings and focuses its first control")
    settingsButton.performClick(nil)
    check(subject.currentView == (connected ? "connected" : "idle") &&
          subject.window.firstResponder === settingsButton &&
          subject.controls["done"]?.isHidden == true && subject.window.frame == frame,
          "Clicking Settings again returns to the main screen and restores focus without moving the window")
}
subject.status["ready"] = false
subject.currentView = "settings"
subject.render()
(subject.controls["settings"] as? NSButton)?.performClick(nil)
check(subject.currentView == "starting",
      "Closing Settings through its icon preserves startup readiness")
subject.status.removeValue(forKey: "ready")
subject.currentView = "idle"
subject.render()
check(subject.controls["token"] == nil && subject.controls["tokenView"] == nil && subject.controls["login"] == nil,
      "Access-token sign-in is absent from the native UI")
subject.currentView = "connected"
subject.status = ["connected": true, "username": "example", "claudeEnabled": true, "codexEnabled": false,
                  "copilotEnabled": true, "copilotURL": "http://127.0.0.1:18925/v1", "version": "2.1.7"]
subject.render()
check((subject.controls["copilot"] as? NSButton)?.accessibilityRole() == .checkBox &&
      (subject.controls["copilot"] as? NSButton)?.accessibilityLabel() == "GitHub Copilot" &&
      (subject.controls["copilot"] as? NSButton)?.state == .on,
      "Copilot Bridge exposes its native checkbox name and enabled state")
check(subject.controls["copilotCopy"] == nil, "The Copilot setup copy action is absent")
check(subject.controls["usageLabel"]?.isHidden == true && subject.controls["usageValue"]?.isHidden == true,
      "Unavailable usage is hidden rather than displaying zero credits")
for appearance in ["light", "dark"] {
    subject.status["appearance"] = appearance
    subject.render()
    let colors = NativeColors.current
    for color in [colors.accent, colors.secondary, colors.on] {
        let rgb = color.usingColorSpace(.sRGB)!
        check(rgb.redComponent > rgb.blueComponent && rgb.redComponent > rgb.greenComponent,
              "The \(appearance) theme uses Pomegram brand and control colors")
    }
    let green = colors.connected.usingColorSpace(.sRGB)!
    let gray = colors.disconnected.usingColorSpace(.sRGB)!
    let red = colors.danger.usingColorSpace(.sRGB)!
    check(green.greenComponent > green.redComponent && green.greenComponent > green.blueComponent &&
          red.redComponent > red.greenComponent && red.redComponent > red.blueComponent &&
          abs(gray.redComponent - gray.greenComponent) < 0.1 && abs(gray.greenComponent - gray.blueComponent) < 0.1,
          "The \(appearance) theme preserves green, red, and neutral gray functional colors")
    for (key, title) in [("codex", "Codex"), ("claude", "Claude Code"), ("copilot", "GitHub Copilot")] {
        let button = subject.controls[key] as! NativeButton
        check(button.title == title && button.accessibilityLabel() == title,
              "The \(title) row and accessible name omit Bridge")
        let bitmap = button.bitmapImageRepForCachingDisplay(in: button.bounds)!
        button.cacheDisplay(in: button.bounds, to: bitmap)
        let srgb = bitmap.converting(to: .sRGB, renderingIntent: .relativeColorimetric)!
        let scale = CGFloat(srgb.pixelsWide) / button.bounds.width
        let dot = srgb.colorAt(x: Int(62 * scale), y: Int(37 * scale))!
        let expected = button.state == .on ? green : gray
        check(abs(dot.redComponent - expected.redComponent) < 0.02 &&
              abs(dot.greenComponent - expected.greenComponent) < 0.02 &&
              abs(dot.blueComponent - expected.blueComponent) < 0.02,
              "The \(appearance) \(title) status dot renders its functional color")
    }
    for (foreground, background) in [
        (colors.text, colors.background), (colors.muted, colors.panel),
        (colors.accent, colors.badge), (colors.secondary, colors.background),
        (colors.connected, colors.panel), (colors.disconnected, colors.button),
        (NSColor.white, colors.on)
    ] {
        check(contrast(foreground, background) >= 4.5,
              "The \(appearance) theme preserves at least 4.5:1 text contrast")
    }
    for key in ["connect"] {
        subject.currentView = key == "disconnect" ? "connected" : key == "login" ? "token" : "idle"
        subject.render()
        let button = subject.controls[key] as! NativeButton
        button.hover = true
        let bitmap = button.bitmapImageRepForCachingDisplay(in: button.bounds)!
        button.cacheDisplay(in: button.bounds, to: bitmap)
        let pixels = bitmap.converting(to: .sRGB, renderingIntent: .relativeColorimetric)!
        let scale = CGFloat(pixels.pixelsWide) / button.bounds.width
        let background = pixels.colorAt(x: Int(10 * scale), y: Int(10 * scale))!
        check(contrast(key == "disconnect" ? colors.connected : colors.disconnected, background) >= 4.5,
              "The \(appearance) \(key) hover preserves semantic text color and 4.5:1 contrast")
        button.hover = false
    }
    subject.currentView = "connected"
    subject.render()
    for (foreground, background) in [
        (colors.on, colors.panel), (colors.track, colors.panel),
        (NSColor.white, colors.on), (NSColor.white, colors.track)
    ] {
        check(contrast(foreground, background) >= 3,
              "The \(appearance) theme preserves at least 3:1 control contrast")
    }
    let top = subject.window.frame.maxY
    for _ in 0..<3 {
        subject.render()
        check(subject.window.contentView?.bounds.size == windowSize &&
              subject.controls["account"]?.frame.minY == 8 &&
              subject.controls["codex"]?.frame.minY == 109 &&
              subject.controls["project"] == nil &&
              subject.controls["notice"]?.isHidden == true,
              "The \(appearance) homepage keeps its compact fixed layout")
    }
    check(subject.controls["codex"]!.frame.minY > mainGlassY &&
          subject.controls["claude"]!.frame.minY - subject.controls["codex"]!.frame.minY == 44 &&
          subject.controls["copilot"]!.frame.minY - subject.controls["claude"]!.frame.minY == 44,
          "Three aligned client rows have equal spacing in the glass area")
    let frames = subject.controls.mapValues { $0.frame }
    for source in ["notice", "error"] {
        if source == "notice" { subject.notice = "Could not save settings." }
        else { subject.status["error"] = "GitHub needs authorization." }
        subject.render()
        check(subject.window.contentView?.bounds.size == windowSize &&
              subject.controls["notice"]?.isHidden == false &&
              subject.controls["notice"]!.frame == NSRect(x: 20, y: 56, width: 360, height: 42) &&
              subject.controls["title"] == nil &&
              subject.controls["notice"]!.frame.minY >= subject.controls["disconnect"]!.frame.maxY &&
              ["account", "claude", "codex", "copilot", "settings"].allSatisfy {
                  subject.controls[$0]!.frame == frames[$0]
              },
              "The \(appearance) \(source) uses the header without moving cards or covering header actions")
        captureWindow(subject.window, "\(appearance)-\(source)")
        subject.notice = ""
        subject.status.removeValue(forKey: "error")
        subject.render()
    }
    check(subject.window.frame.maxY == top, "Status changes preserve the window's top edge")
    captureWindow(subject.window, "\(appearance)-connected")
    subject.busy = true
    subject.pendingBridge = "codex"
    subject.render()
    check(subject.window.contentView?.bounds.size == windowSize &&
          (subject.controls["codex"] as? NativeButton)?.bridgeStatusText == "Working..." &&
          subject.controls["notice"]?.isHidden == true,
          "The \(appearance) pending state replaces the row status without a layout shift")
    captureWindow(subject.window, "\(appearance)-working")
    subject.busy = false
    subject.pendingBridge = nil
    subject.status["copilotEnabled"] = false
    subject.render()
    let copilot = subject.controls["copilot"] as! NativeButton
    check(copilot.state == .off && copilot.bridgeStatusText == "Paused" &&
          copilot.accessibilityHelp() == copilot.bridgeStatusText,
          "Paused Copilot matches Codex, including accessibility")
    let guidance = (copilot.bridgeStatusText as NSString).size(withAttributes: [.font: copilot.bridgeStatusFont])
    check(guidance.width <= 180 && guidance.height <= 20,
          "Copilot status fits on one line without overlapping its switch")
    captureWindow(subject.window, "\(appearance)-copilot-paused")
    check(subject.window.contentView!.subviews.filter { !$0.isHidden }.allSatisfy {
        subject.window.contentView!.bounds.contains($0.frame)
    }, "All visible homepage controls fit without clipping")
    let mainFrame = subject.window.frame
    for _ in 0..<3 {
        subject.settingsView()
        check(subject.window.frame == mainFrame && subject.controls["project"] == nil,
              "Settings keeps the exact main window size, position, and footer")
        subject.done()
        check(subject.window.frame == mainFrame,
              "Returning from Settings does not resize or move the main window")
    }
    subject.settingsView()
    check(subject.controls["quit"] == nil && subject.controls["project"] == nil,
          "Settings has no quit link or product version")
    captureWindow(subject.window, "\(appearance)-settings")
    for view in ["idle", "device", "settings"] {
        for error in ["", "Could not complete this action. Try again."] {
            subject.currentView = view
            subject.notice = error
            subject.render()
            let content = subject.window.contentView!
            check(content.subviews.filter { !$0.isHidden }.allSatisfy {
                content.bounds.contains($0.frame)
            }, "The \(appearance) \(view) controls and errors fit without clipping")
        }
    }
    subject.notice = ""
    subject.currentView = "connected"
    subject.render()
}
subject.status["username"] = "xjin6"
subject.status["version"] = "2.1.6"
subject.render()
check(subject.controls["project"] == nil, "Status updates do not restore a version label")
check((subject.controls["account"] as? NSTextField)?.stringValue == "@xjin6",
      "Removing author attribution preserves the signed-in username")

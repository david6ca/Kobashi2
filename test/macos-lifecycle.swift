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
check(subject.createWindow(), "The native window loads its existing brand artwork")
check(subject.window.title == "Kobashi2" &&
      (subject.controls["title"] as? NSTextField)?.stringValue == "Kobashi2" &&
      subject.statusItem?.button?.toolTip == "Kobashi2" &&
      subject.statusItem?.button?.accessibilityLabel() == "Kobashi2",
      "Window, header, and menu-bar accessibility use Kobashi2")
check(application.mainMenu?.items.first?.submenu?.items.first?.title == "About Kobashi2" &&
      application.mainMenu?.items.first?.submenu?.items.last?.title == "Quit Kobashi2" &&
      subject.statusItem?.menu?.items.last?.title == "Quit Kobashi2",
      "Application and menu-bar menus use the renamed app")
let heading = subject.controls["title"] as! NSTextField
check(("Kobashi2" as NSString).size(withAttributes: [.font: heading.font!]).width <= heading.bounds.width,
      "The Kobashi2 heading fits without clipping")
check(subject.controls["startupHelp"] == nil && subject.controls["trayHelp"] == nil &&
      subject.controls["theme"] == nil,
      "Settings descriptions and the header theme toggle are removed")
check((subject.controls["connect"] as? NSButton)?.title == "Starting..." &&
      (subject.controls["connect"] as? NSButton)?.isEnabled == false,
      "Startup waits for the local bridge instead of offering sign-in")
subject.currentView = "idle"
subject.render()
check(subject.window.contentView?.bounds.size == NSSize(width: 400, height: 390),
      "The compact window has a fixed 400 by 390 content area")
check(!subject.window.styleMask.contains(.resizable), "The window cannot be resized")
check(subject.statusItem?.menu?.items.first?.title == "Show Kobashi2",
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
check(subject.controls["token"] is NSSecureTextField,
      "Token entry uses a native protected text field")
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
check(subject.controls["settings"]?.frame == NSRect(x: 336, y: 24, width: 32, height: 32),
      "Settings is the only header action, aligned to the right padding")
check(subject.controls["subtitle"] == nil, "The route description is absent")
check(NSImage(systemSymbolName: "gearshape", accessibilityDescription: nil) != nil,
      "The standard Settings gear is available")
for (key, title, action) in [
    ("connect", "Not Connected", #selector(AppDelegate.connect)),
    ("login", "Not Connected", #selector(AppDelegate.login)),
    ("disconnect", "Connected", #selector(AppDelegate.disconnect))
] {
    check((subject.controls[key] as? NSButton)?.title == title &&
          (subject.controls[key] as? NSButton)?.action == action &&
          (subject.controls[key] as? NSButton)?.accessibilityLabel() == title &&
          (subject.controls[key] as? NSButton)?.accessibilityRole() == .button,
          "The \(key) status retains its native accessible name and action")
}
check(subject.controls["claude"]?.frame == NSRect(x: 40, y: 184, width: 320, height: 50) &&
      subject.controls["codex"]?.frame == NSRect(x: 40, y: 234, width: 320, height: 50) &&
      subject.controls["copilot"]?.frame == NSRect(x: 40, y: 284, width: 320, height: 50),
      "The three Bridge rows share one padded group in the same order")
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
if let hint = subject.controls["tokenTeaser"] as? NSTextField,
   let link = subject.controls["tokenView"] as? NSButton {
    let font = NSFont.systemFont(ofSize: 12)
    check(hint.frame.width >= (hint.stringValue as NSString).size(withAttributes: [.font: font]).width + 4 &&
          link.frame.width >= (link.title as NSString).size(withAttributes: [.font: font]).width + 4,
          "The token hint and link fit their native font metrics")
    check(hint.frame.minX >= 32 && hint.frame.maxX + 6 <= link.frame.minX && link.frame.maxX <= 368,
          "The single-line token prompt stays separated within the content padding")
} else {
    check(false, "The token prompt retains native label and button controls")
}
subject.tokenView()
check(subject.controls["connect"]?.isHidden == false && subject.controls["token"]?.isHidden == false,
      "Token entry expands within the original sign-in screen")
check((subject.controls["login"] as? NSButton)?.isEnabled == false,
      "Connect remains disabled for an empty token")
subject.currentView = "connected"
subject.status = ["connected": true, "username": "example", "claudeEnabled": true, "codexEnabled": false,
                  "copilotEnabled": true, "copilotURL": "http://127.0.0.1:18925/v1", "version": "2.1.7"]
subject.render()
check((subject.controls["copilot"] as? NSButton)?.accessibilityRole() == .checkBox &&
      (subject.controls["copilot"] as? NSButton)?.accessibilityLabel() == "Copilot" &&
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
        check(rgb.blueComponent > rgb.redComponent && rgb.redComponent > rgb.greenComponent,
              "The \(appearance) theme uses purple brand and control colors")
    }
    let green = colors.connected.usingColorSpace(.sRGB)!
    let gray = colors.disconnected.usingColorSpace(.sRGB)!
    let red = colors.danger.usingColorSpace(.sRGB)!
    check(green.greenComponent > green.redComponent && green.greenComponent > green.blueComponent &&
          red.redComponent > red.greenComponent && red.redComponent > red.blueComponent &&
          abs(gray.redComponent - gray.greenComponent) < 0.1 && abs(gray.greenComponent - gray.blueComponent) < 0.1,
          "The \(appearance) theme preserves green, red, and neutral gray functional colors")
    for (key, title) in [("claude", "Claude"), ("codex", "Codex"), ("copilot", "Copilot")] {
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
    for key in ["connect", "login", "disconnect"] {
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
        check(subject.window.contentView?.bounds.size == NSSize(width: 400, height: 390) &&
              subject.controls["account"]?.frame.minY == 92 &&
              subject.controls["claude"]?.frame.minY == 184 &&
              subject.controls["project"]?.frame.minY == 356 &&
              subject.controls["notice"]?.isHidden == true,
              "The \(appearance) homepage keeps its compact fixed layout")
    }
    let panelTop = subject.controls["claude"]!.frame.minY - 6
    let panelBottom = subject.controls["copilot"]!.frame.maxY + 6
    check(72 - subject.controls["settings"]!.frame.maxY == 16 &&
          panelTop - 162 == 16 && subject.controls["project"]!.frame.minY - panelBottom == 16 &&
          subject.controls["codex"]!.frame.minY == subject.controls["claude"]!.frame.maxY &&
          subject.controls["copilot"]!.frame.minY == subject.controls["codex"]!.frame.maxY,
          "The header, account, client panel, and version have equal 16-point gaps")
    let content = subject.window.contentView!
    let bitmap = content.bitmapImageRepForCachingDisplay(in: content.bounds)!
    content.cacheDisplay(in: content.bounds, to: bitmap)
    let pixels = bitmap.converting(to: .sRGB, renderingIntent: .relativeColorimetric)!
    let scale = CGFloat(pixels.pixelsWide) / content.bounds.width
    func matches(_ x: Int, _ y: Int, _ color: NSColor) -> Bool {
        let actual = pixels.colorAt(x: Int(CGFloat(x) * scale), y: Int(CGFloat(y) * scale))!
        let expected = color.usingColorSpace(.sRGB)!
        return abs(actual.redComponent - expected.redComponent) < 0.02 &&
            abs(actual.greenComponent - expected.greenComponent) < 0.02 &&
            abs(actual.blueComponent - expected.blueComponent) < 0.02
    }
    check([38, 44, 50].allSatisfy { matches($0, 38, colors.accent) } &&
          [38, 50].allSatisfy { matches($0, 35, colors.accent) } &&
          matches(44, 35, colors.background) && matches(48, 29, colors.background),
          "The \(appearance) header renders two solid circles joined by a line, not a chevron")
    check((56..<72).allSatisfy { matches(200, $0, colors.background) } &&
          (162..<178).allSatisfy { matches(200, $0, colors.background) } &&
          (340..<356).allSatisfy { matches(200, $0, colors.background) },
          "The \(appearance) rendered header, panel, and footer gaps are all exactly 16 points")
    let frames = subject.controls.mapValues { $0.frame }
    for source in ["notice", "error"] {
        if source == "notice" { subject.notice = "Could not save settings." }
        else { subject.status["error"] = "GitHub needs authorization." }
        subject.render()
        check(subject.window.contentView?.bounds.size == NSSize(width: 400, height: 390) &&
              subject.controls["notice"]?.isHidden == false &&
              subject.controls["notice"]!.frame == NSRect(x: 32, y: 24, width: 248, height: 32) &&
              subject.controls["title"]?.isHidden == true &&
              subject.controls["notice"]!.frame.maxX < subject.controls["settings"]!.frame.minX &&
              ["account", "claude", "codex", "copilot", "project"].allSatisfy {
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
    check(subject.window.contentView?.bounds.size == NSSize(width: 400, height: 390) &&
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
        check(subject.window.frame == mainFrame && subject.controls["project"]?.frame.minY == 356,
              "Settings keeps the exact main window size, position, and footer")
        subject.done()
        check(subject.window.frame == mainFrame,
              "Returning from Settings does not resize or move the main window")
    }
    subject.settingsView()
    check(subject.controls["quit"] == nil &&
          subject.controls["project"]!.frame.minY - subject.controls["done"]!.frame.maxY == 16,
          "Settings has no quit link and preserves the same footer spacing")
    subject.window.recalculateKeyViewLoop()
    subject.window.makeFirstResponder(subject.controls["done"])
    subject.window.selectNextKeyView(nil)
    check(subject.window.firstResponder === subject.controls["project"],
          "Tab reaches Version, not \((subject.window.firstResponder as? NativeButton)?.key ?? String(describing: subject.window.firstResponder))")
    check((subject.controls["project"] as? NativeButton)?.showsKeyboardFocus == true,
          "Tab reaches Version with a visible keyboard focus indicator")
    subject.window.selectPreviousKeyView(nil)
    check(subject.window.firstResponder === subject.controls["done"] &&
          (subject.controls["done"] as? NativeButton)?.showsKeyboardFocus == true,
          "Shift-Tab returns to Done with a visible keyboard focus indicator")
    subject.window.makeFirstResponder(subject.controls["tray"])
    subject.window.selectNextKeyView(nil)
    check(subject.window.firstResponder === appearanceSelector,
          "Tab reaches the native Appearance selector after Close to system tray")
    subject.window.selectNextKeyView(nil)
    check(subject.window.firstResponder === subject.controls["done"],
          "Tab continues from Appearance to Done")
    subject.window.selectPreviousKeyView(nil)
    check(subject.window.firstResponder === appearanceSelector,
          "Shift-Tab returns from Done to Appearance")
    check(!appearanceSelector.isHidden && appearanceSelector.selectedItem?.title == appearance.capitalized &&
          !appearanceSelector.frame.intersects(subject.controls["appearanceLabel"]!.frame),
          "The \(appearance) Settings selector reflects the confirmed mode without overlapping its label")
    for key in ["startup", "tray"] {
        let button = subject.controls[key] as! NativeButton
        check(!button.frame.intersects(appearanceSelector.frame) &&
              !button.frame.intersects(subject.controls["appearanceLabel"]!.frame),
              "The \(key) checkbox cannot overlap Appearance")
        button.showsKeyboardFocus = false
        subject.window.makeFirstResponder(button)
        button.display()
        check(!button.showsKeyboardFocus, "Direct selection of \(key) adds no thick row border")
        if button.state == .on {
            let checkbox = button.bitmapImageRepForCachingDisplay(in: button.bounds)!
            button.cacheDisplay(in: button.bounds, to: checkbox)
            let srgb = checkbox.converting(to: .sRGB, renderingIntent: .relativeColorimetric)!
            let scale = CGFloat(checkbox.pixelsWide) / button.bounds.width
            let sample = srgb.colorAt(x: Int(312 * scale), y: Int(10 * scale))!
            let fill = NSColor(srgbRed: sample.redComponent, green: sample.greenComponent,
                               blue: sample.blueComponent, alpha: 1)
            check(contrast(.white, fill) >= 4.5,
                  "The \(appearance) checked box keeps its white tick legible")
        }
    }
    captureWindow(subject.window, "\(appearance)-settings")
    for view in ["idle", "token", "device", "settings"] {
        for error in ["", "Could not complete this action. Try again."] {
            subject.currentView = view
            subject.notice = error
            subject.render()
            let content = subject.window.contentView!
            let footer = subject.controls["project"]!.frame
            check(content.subviews.filter { !$0.isHidden }.allSatisfy {
                content.bounds.contains($0.frame) &&
                    ($0 === subject.controls["project"] || !$0.frame.intersects(footer))
            }, "The \(appearance) \(view) controls and errors fit above the anchored version footer")
        }
    }
    subject.notice = ""
    subject.currentView = "connected"
    subject.render()
}
subject.status["username"] = "xjin6"
subject.status["version"] = "2.1.6"
subject.render()
check((subject.controls["project"] as? NSButton)?.title == "v2.1.6" &&
      (subject.controls["project"] as? NSButton)?.accessibilityRole() == .button,
      "The version link retains a native accessible name without author attribution")
check((subject.controls["account"] as? NSTextField)?.stringValue == "@xjin6",
      "Removing author attribution preserves the signed-in username")

func check(_ condition: @autoclosure () -> Bool, _ message: String) {
    guard condition() else {
        fputs("FAIL: \(message)\n", stderr)
        exit(1)
    }
    print("PASS: \(message)")
}

class ToggleCheckDelegate: AppDelegate {
    var requests: [String] = []
    var enabled = ["codexEnabled": true, "claudeEnabled": true, "copilotEnabled": true]
    var step = 0
    var nextError: String?
    let clicks = [
        ("codex", NSPoint(x: 295, y: 25)),
        ("codex", NSPoint(x: 168, y: 25)),
        ("claude", NSPoint(x: 295, y: 25)),
        ("claude", NSPoint(x: 168, y: 25)),
        ("copilot", NSPoint(x: 295, y: 25)),
        ("copilot", NSPoint(x: 168, y: 25))
    ]

    override func applicationDidFinishLaunching(_ notification: Notification) {
        check(createWindow(), "The isolated toggle window loads")
        currentView = "connected"
        status = ["connected": true, "codexEnabled": true, "claudeEnabled": true, "copilotEnabled": true,
                  "copilotURL": "http://127.0.0.1:18925/v1"]
        render()
        NSApp.activate(ignoringOtherApps: true)
        DispatchQueue.main.asyncAfter(deadline: .now() + 1) { self.clickNext() }
    }

    override func request(_ path: String, body: [String: Any]? = nil,
                          completion: @escaping ([String: Any]?, String?) -> Void) {
        requests.append(path)
        if path == "/api/settings" {
            check(pendingBridge == nil && notice.isEmpty && controls["settingsTitle"]?.isHidden == false,
                  "Saving settings keeps its title and controls in place")
            DispatchQueue.main.async { completion(["startAtLogin": true, "closeToTray": true], nil) }
            return
        }
        let key = String(path.dropFirst("/api/toggle-".count))
        check(pendingBridge == key && busy && notice.isEmpty && controls["notice"]?.isHidden == true,
              "The \(key) toggle has no progress banner")
        check(window.contentView?.bounds.size == NSSize(width: 400, height: 390) &&
              controls["claude"]?.frame.minY == 184 && controls["codex"]?.frame.minY == 234 &&
              controls["copilot"]?.frame.minY == 284,
              "The pending \(key) toggle does not move or resize the homepage")
        for other in ["codex", "claude", "copilot"] {
            let button = controls[other] as! NativeButton
            let expected = other == key ? "Working..." : enabled["\(other)Enabled"] == true ? "Active" : "Paused"
            check(button.bridgeStatusText == expected && button.accessibilityHelp() == expected,
                  "Only the pending bridge reports Working, including accessibility")
        }
        if let error = nextError {
            nextError = nil
            DispatchQueue.main.async { completion(nil, error) }
            return
        }
        let field = String(path.dropFirst("/api/toggle-".count)) + "Enabled"
        enabled[field] = !(enabled[field] ?? false)
        var response: [String: Any] = enabled
        response["notice"] = step.isMultiple(of: 2)
            ? "Kobashi provider registered and selected for the current local session."
            : "Kobashi registered; no session was switched. Select its model when idle."
        DispatchQueue.main.async { completion(response, nil) }
    }

    override func refresh() {}
    override func showError(_ message: String) {
        check(message == "Could not enable bridge.", "Toggle failures retain their actionable error")
    }

    func clickNext() {
        guard step < clicks.count else {
            nextError = "Could not enable bridge."
            action("/api/toggle-codex")
            DispatchQueue.main.async {
                check(!self.busy && self.pendingBridge == nil &&
                      self.notice == "Could not enable bridge." && self.controls["notice"]?.isHidden == false,
                      "A failed toggle clears Working and keeps its error visible")
                check((self.controls["codex"] as? NativeButton)?.state == .on,
                      "A failed toggle preserves the confirmed state (control: \((self.controls["codex"] as? NativeButton)?.state.rawValue ?? -1), status: \(String(describing: self.status["codexEnabled"])), confirmed: \(String(describing: self.enabled["codexEnabled"])))")
                self.settingsView()
                self.saveStartup()
                DispatchQueue.main.async {
                    check(self.notice.isEmpty && self.controls["settingsTitle"]?.isHidden == false &&
                          self.controls["startupHelp"] == nil && self.controls["trayHelp"] == nil &&
                          self.controls["appearance"]?.isHidden == false,
                          "A successful settings change adds no saved notice")
                    print("PASS: All bridge cards toggle off and back on through mouse events")
                    exit(0)
                }
            }
            return
        }
        let (key, point) = clicks[step]
        let button = controls[key] as! NativeButton
        let location = button.convert(point, to: nil)
        check(window.contentView?.hitTest(location) === button,
              "The \(key) switch hit-tests to its bridge card")
        let count = requests.count
        let timestamp = ProcessInfo.processInfo.systemUptime
        let down = NSEvent.mouseEvent(with: .leftMouseDown, location: location, modifierFlags: [],
                                     timestamp: timestamp, windowNumber: window.windowNumber,
                                     context: nil, eventNumber: step * 2 + 1, clickCount: 1, pressure: 1)!
        let up = NSEvent.mouseEvent(with: .leftMouseUp, location: location, modifierFlags: [],
                                   timestamp: timestamp + 0.01, windowNumber: window.windowNumber,
                                   context: nil, eventNumber: step * 2 + 2, clickCount: 1, pressure: 0)!
        NSApp.postEvent(down, atStart: false)
        NSApp.postEvent(up, atStart: false)
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) {
            check(self.requests.count == count + 1 && self.requests.last == "/api/toggle-\(key)",
                  "Clicking the \(key) card at \(point) sends exactly one toggle request")
            check(button.state == (self.enabled["\(key)Enabled"] == true ? .on : .off),
                  "The \(key) card displays the confirmed bridge state")
            let expected = button.state == .on ? "Active" : "Paused"
            check(button.bridgeStatusText == expected, "The \(key) bridge shows \(expected)")
            check(self.notice.isEmpty && self.controls["notice"]?.isHidden == true && self.pendingBridge == nil,
                  "Successful toggles show no provider-registration explanation")
            let textSize = (button.bridgeStatusText as NSString).size(withAttributes: [
                .font: button.bridgeStatusFont
            ])
            check(textSize.width <= 180 && textSize.height <= 20 &&
                  74 + 180 + 10 <= button.bounds.width - 55,
                  "The \(key) status fits without clipping or overlapping its toggle")
            check(button.accessibilityLabel() == button.title && button.acceptsFirstResponder,
                  "The \(key) bridge retains its accessible name and keyboard focus")
            check(!button.showsKeyboardFocus, "Mouse selection of \(key) adds no row focus border")
            self.step += 1
            DispatchQueue.main.asyncAfter(deadline: .now() + NSEvent.doubleClickInterval + 0.02) {
                self.clickNext()
            }
        }
    }
}

let application = NSApplication.shared
application.setActivationPolicy(.regular)
let toggles = ToggleCheckDelegate()
application.delegate = toggles
DispatchQueue.main.asyncAfter(deadline: .now() + 15) {
    check(false, "The isolated mouse tests complete before the timeout")
}
application.run()

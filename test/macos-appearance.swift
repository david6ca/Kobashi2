func check(_ condition: @autoclosure () -> Bool, _ message: String) {
    guard condition() else {
        fputs("FAIL: \(message)\n", stderr)
        exit(1)
    }
    print("PASS: \(message)")
}

class AppearanceCheckDelegate: AppDelegate {
    var requests: [String] = []
    var nextError: String?
    var reportedError = ""
    var originalFrame = NSRect.zero

    var selector: NSPopUpButton { controls["appearance"] as! NSPopUpButton }

    override func applicationDidFinishLaunching(_ notification: Notification) {
        check(createWindow(), "The isolated appearance window loads")
        status = ["ready": true, "connected": false, "appearance": "system"]
        settingsView()
        originalFrame = window.frame
        DispatchQueue.main.async { self.chooseNext(0) }
    }

    override func request(_ path: String, body: [String: Any]? = nil,
                          completion: @escaping ([String: Any]?, String?) -> Void) {
        requests.append(path)
        check(path == "/api/appearance", "Appearance never changes login or client configuration")
        check(busy && notice.isEmpty && controls["settingsTitle"]?.isHidden == false && !selector.isEnabled,
              "Saving appearance is silent and disables only the pending interaction")
        check(window.frame == originalFrame && controls["project"]?.frame.minY == 356,
              "Saving appearance does not resize Settings or move the footer")
        if let error = nextError {
            nextError = nil
            DispatchQueue.main.async { completion(nil, error) }
        } else {
            DispatchQueue.main.async { completion(["appearance": body!["appearance"]!], nil) }
        }
    }

    override func refresh() {}
    override func showError(_ message: String) { reportedError = message }

    func chooseNext(_ step: Int) {
        let modes = ["light", "dark", "system"]
        guard step < modes.count else {
            let count = requests.count
            selector.selectItem(at: 0)
            selector.sendAction(selector.action, to: selector.target)
            check(requests.count == count && !busy, "Selecting the saved Auto choice makes no redundant request")
            nextError = "Could not save appearance."
            selector.selectItem(at: 2)
            selector.sendAction(selector.action, to: selector.target)
            DispatchQueue.main.async {
                check(!self.busy && self.selector.isEnabled && self.selector.selectedItem?.title == "Auto" &&
                      self.status["appearance"] as? String == "system" && NSApp.appearance == nil,
                      "A failed appearance change restores the saved selection and system following")
                check(self.notice == "Could not save appearance." && self.reportedError == self.notice &&
                      self.controls["notice"]?.isHidden == false,
                      "Appearance persistence failures remain visible and actionable")
                self.done()
                check(self.selector.isHidden && self.controls["appearanceLabel"]?.isHidden == true &&
                      self.controls["theme"] == nil && self.window.frame == self.originalFrame,
                      "Appearance controls stay inside Settings without changing the main window")
                print("PASS: Auto, Light, Dark and failed saves preserve native settings behavior")
                exit(0)
            }
            return
        }
        let mode = modes[step]
        window.makeFirstResponder(selector)
        selector.selectItem(at: ["system", "light", "dark"].firstIndex(of: mode)!)
        check(selector.sendAction(selector.action, to: selector.target), "The native selector invokes its save action")
        DispatchQueue.main.async {
            check(!self.busy && self.selector.isEnabled && self.status["appearance"] as? String == mode &&
                  self.selector.selectedItem?.representedObject as? String == mode,
                  "The \(mode) choice applies and remains selected after saving")
            if mode == "system" {
                check(NSApp.appearance == nil && self.selector.selectedItem?.title == "Auto",
                      "Auto removes the override and follows macOS appearance")
            } else {
                check(NSApp.appearance?.name == (mode == "dark" ? .darkAqua : .aqua),
                      "The \(mode) choice explicitly overrides macOS appearance")
            }
            check(self.notice.isEmpty && self.controls["settingsTitle"]?.isHidden == false &&
                  self.window.firstResponder === self.selector && self.window.frame == self.originalFrame,
                  "Appearance saves preserve focus, title and layout without success notices")
            self.chooseNext(step + 1)
        }
    }
}

let application = NSApplication.shared
application.setActivationPolicy(.regular)
let subject = AppearanceCheckDelegate()
application.delegate = subject
DispatchQueue.main.asyncAfter(deadline: .now() + 15) {
    check(false, "Appearance checks complete before the timeout")
}
application.run()

func check(_ condition: @autoclosure () -> Bool, _ message: String) {
    guard condition() else {
        fputs("FAIL: \(message)\n", stderr)
        exit(1)
    }
    print("PASS: \(message)")
}

class ContextCheckDelegate: AppDelegate {
    var requests: [(String, [String: Any])] = []
    var saved = ["qwen3-4b": 32768, "qwen3-8b": 40960, "qwen3-coder-30b": 16384]
    var failSave = false
    var failLoad = false
    var reportedErrors: [String] = []
    var fixture: [String: Any] = ["ready": true, "connected": true, "localModel": "qwen3-4b",
                                  "authMethod": "local", "username": "Qwen3 4B", "version": "test"]

    override func applicationDidFinishLaunching(_ notification: Notification) {
        check(createWindow(), "The isolated context window loads without a bridge")
        status = fixture
        currentView = "connected"
        render()
        window.makeFirstResponder(controls["settings"])
        loadLocalContext()
        later { self.checkSlider() }
    }
    override func request(_ path: String, body: [String: Any]? = nil,
                          completion: @escaping ([String: Any]?, String?) -> Void) {
        if let body { requests.append((path, body)) }
        var result: [String: Any] = [:]
        var error: String?
        switch path {
        case "/api/status": result = fixture
        case "/api/settings": result = ["closeToTray": true]
        case "/api/local/models":
            if failLoad {
                error = "Could not load local context settings."
                failLoad = false
            } else {
                result = ["supported": true, "models": [
                    ["id": "qwen3-4b", "name": "Qwen3 4B", "contextWindow": saved["qwen3-4b"]!,
                     "activeContextWindow": saved["qwen3-4b"]!, "contextPending": false,
                     "recommendedContextWindow": 131072,
                     "contextChoices": [8192, 16384, 32768, 65536, 131072, 262144]],
                    ["id": "qwen3-8b", "name": "Qwen3 8B", "contextWindow": saved["qwen3-8b"]!,
                     "activeContextWindow": saved["qwen3-8b"]!, "contextPending": false,
                     "recommendedContextWindow": 40960,
                     "contextChoices": [8192, 16384, 32768, 40960]],
                    ["id": "qwen3-coder-30b", "name": "Qwen3-Coder 30B", "contextWindow": 16384,
                     "activeContextWindow": 16384, "contextPending": false,
                     "recommendedContextWindow": 16384,
                     "contextChoices": [8192, 16384, 32768, 65536, 131072, 262144]]
                ]]
            }
        case "/api/local/context":
            if failSave {
                error = "Could not save local context settings."
                failSave = false
            } else {
                let id = body!["model"] as! String
                let value = body!["contextWindow"] as! Int
                saved[id] = value
                result = ["model": id, "contextWindow": value,
                          "activeContextWindow": value, "contextPending": false]
            }
        default: check(false, "Unexpected context mock request \(path)")
        }
        DispatchQueue.main.async { completion(result, error) }
    }
    override func showError(_ message: String) { reportedErrors.append(message) }
    func later(_ body: @escaping () -> Void) {
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.15, execute: body)
    }
    func key(_ code: UInt16, _ characters: String) {
        let event = NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: [],
                                    timestamp: ProcessInfo.processInfo.systemUptime,
                                    windowNumber: window.windowNumber, context: nil, characters: characters,
                                    charactersIgnoringModifiers: characters, isARepeat: false, keyCode: code)!
        window.sendEvent(event)
    }
    var slider: NSSlider { controls["contextSlider"] as! NSSlider }
    var usageText: String { (controls["usageValue"] as! NSTextField).stringValue }
    func checkSlider() {
        check(currentView == "connected" && window.firstResponder === controls["settings"] &&
              slider.isEnabled && !slider.isHidden,
              "Context loads on the main screen without navigating or stealing focus")
        check(slider.frame == NSRect(x: 50, y: 137, width: 299, height: 24) &&
              controls["usageLabel"]?.frame.minY == 120 &&
              controls["codex"]?.frame.minY == 184 && controls["project"]?.frame.minY == footerY &&
              window.contentView?.bounds.size == windowSize,
              "The slider occupies the token-usage area inside the original account card; client cards and footer stay fixed")
        check(slider.numberOfTickMarks == 6 && slider.integerValue == 2 &&
              !slider.isContinuous && slider.allowsTickMarkValuesOnly,
              "The native slider uses discrete capacities and saves only committed changes")
        check(slider.accessibilityLabel() == "Context window" &&
              slider.accessibilityValueDescription() == "32,768 tokens" &&
              slider.accessibilityHelp()?.contains("Left and Right Arrow") == true &&
              slider.accessibilityHelp()?.contains("262,144 tokens") == true && usageText == "32,768 tokens",
              "Token value, supported range, and keyboard instructions are accessible")
        check(slider.accessibilityHelp()?.contains("Recommended for your RAM: 131,072 tokens") == true &&
              slider.integerValue == 2,
              "RAM-based guidance is accessible without overriding the saved manual capacity")
        window.makeFirstResponder(slider)
        key(48, "\t")
        check(window.firstResponder === controls["codex"],
              "Tab leaves the slider for the next client control")
        window.makeFirstResponder(slider)
        key(124, "\u{f703}")
        check(busy && notice.isEmpty &&
              (controls["usageLabel"] as! NSTextField).stringValue == "Working...",
              "Keyboard applying uses the existing usage label without a progress banner")
        later { self.checkSavedContext() }
    }
    func checkSavedContext() {
        check(requests.count == 1 && requests[0].0 == "/api/local/context" &&
              requests[0].1["model"] as? String == "qwen3-4b" &&
              requests[0].1["contextWindow"] as? Int == 65536 && saved["qwen3-4b"] == 65536,
              "Right Arrow saves the next capacity for only the connected model")
        check(notice.isEmpty && window.firstResponder === slider &&
              slider.accessibilityValueDescription() == "65,536 tokens" && usageText == "65,536 tokens",
              "Successful saving preserves focus and displays the confirmed capacity")
        check((controls["usageLabel"] as! NSTextField).stringValue == "Restart Codex to apply" &&
              slider.toolTip?.contains("applies immediately") == true &&
              slider.toolTip?.contains("more memory") == true && slider.toolTip?.contains("4-bit") == true,
              "The runtime applies immediately; only the Codex budget reload reminder remains, with memory and KV guidance")
        for appearance in ["light", "dark"] {
            status["appearance"] = appearance
            render()
            check(slider.trackFillColor == NativeColors.current.accent &&
                  slider.frame.maxY <= 162 && window.contentView?.bounds.size == windowSize &&
                  controls["project"]?.frame.minY == footerY,
                  "The \(appearance) slider uses the existing palette and stays inside the account card")
            capture(appearance)
        }
        fixture["localModel"] = "qwen3-8b"
        fixture["username"] = "Qwen3 8B"
        status = fixture
        render()
        check(slider.numberOfTickMarks == 4 && slider.maxValue == 3 &&
              slider.integerValue == 3 && usageText == "40,960 tokens" &&
              (controls["usageLabel"] as! NSTextField).stringValue == "Context · recommended" &&
              slider.toolTip?.contains("Recommended for your RAM: 40,960 tokens") == true,
              "Switching models restores the independent Qwen3 8B capacity and native maximum")
        fixture["localModel"] = "qwen3-4b"
        fixture["username"] = "Qwen3 4B"
        status = fixture
        render()
        check(slider.integerValue == 3 && usageText == "65,536 tokens",
              "Returning to Qwen3 4B restores its confirmed capacity")
        failSave = true
        let knob = (slider.cell as! NSSliderCell).knobRect(flipped: slider.isFlipped)
        let start = slider.convert(NSPoint(x: knob.midX, y: knob.midY), to: nil)
        let end = slider.convert(NSPoint(x: slider.bounds.width * 0.8, y: knob.midY), to: nil)
        for (type, point) in [(NSEvent.EventType.leftMouseDown, start), (.leftMouseDragged, end), (.leftMouseUp, end)] {
            let event = NSEvent.mouseEvent(with: type, location: point, modifierFlags: [],
                                          timestamp: ProcessInfo.processInfo.systemUptime,
                                          windowNumber: window.windowNumber, context: nil, eventNumber: 1,
                                          clickCount: 1, pressure: type == .leftMouseUp ? 0 : 1)!
            NSApp.postEvent(event, atStart: false)
        }
        later { self.checkError() }
    }
    func capture(_ appearance: String) {
        guard let directory = ProcessInfo.processInfo.environment["KOBASHI_CONTEXT_SCREENSHOTS"],
              let view = window.contentView, let image = view.bitmapImageRepForCachingDisplay(in: view.bounds) else { return }
        view.displayIfNeeded()
        view.cacheDisplay(in: view.bounds, to: image)
        guard let data = image.representation(using: .png, properties: [:]) else {
            check(false, "Native context screenshot can be encoded")
            return
        }
        do { try data.write(to: URL(fileURLWithPath: directory).appendingPathComponent("local-context-\(appearance).png")) }
        catch { check(false, "Native context screenshot can be saved: \(error)") }
    }
    func checkError() {
        check(requests.count == 2 && requests.last!.1["contextWindow"] as? Int == 131072 &&
              notice == "Could not save local context settings." && reportedErrors.count == 1 &&
              slider.integerValue == 3 && saved["qwen3-4b"] == 65536 &&
              !(controls["notice"] as! NSTextField).isHidden && controls["title"]!.isHidden,
              "A mouse drag saves once; failed saving preserves capacity and replaces the main title without shifting cards")
        refresh()
        later {
            check(self.notice == "Could not save local context settings." && self.currentView == "connected",
                  "Polling refreshes capacities without erasing a failed setting action")
            self.notice = ""
            self.failLoad = true
            self.loadLocalContext()
            self.later {
                check(self.contextError == "Could not load local context settings." && self.controls["title"]!.isHidden,
                      "Context polling failures are visible without overlapping the main title")
                self.loadLocalContext()
                self.later {
                    check(self.contextError.isEmpty && !self.controls["title"]!.isHidden,
                          "Recovered context polling clears only its own transport error")
                    self.checkSettingsAndCloud()
                }
            }
        }
    }
    func checkSettingsAndCloud() {
        settingsView()
        check(currentView == "settings" && slider.isHidden && !slider.canBecomeKeyView &&
              controls["localContext"] == nil && window.firstResponder === controls["startup"],
              "Settings has no extra context page or hidden focusable slider")
        done()
        check(currentView == "connected" && !slider.isHidden && usageText == "65,536 tokens",
              "Returning from Settings preserves the main-screen context control")
        fixture["authMethod"] = "token"
        fixture["localModel"] = ""
        fixture["username"] = "github-user"
        fixture["usage"] = ["creditsUsed": 25, "entitlement": 100]
        status = fixture
        notice = ""
        window.makeFirstResponder(slider)
        render()
        check(slider.isHidden && !slider.canBecomeKeyView && window.firstResponder !== slider &&
              (controls["usageLabel"] as! NSTextField).stringValue == "AI credits this cycle" &&
              usageText == "25 / 100" && controls["usageLabel"]?.frame.minY == 128,
              "GitHub accounts retain normal credit usage and remove the local slider from focus navigation")
        check(requests.allSatisfy { $0.0 == "/api/local/context" },
              "Context interaction uses the context API without reselecting accounts or restarting apps")
        print("PASS: Native context checks complete without credentials or live bridge ports")
        exit(0)
    }
}

let application = NSApplication.shared
setbuf(stdout, nil)
application.setActivationPolicy(.regular)
let contexts = ContextCheckDelegate()
application.delegate = contexts
DispatchQueue.main.asyncAfter(deadline: .now() + 30) { check(false, "Context checks finish before timeout") }
application.run()

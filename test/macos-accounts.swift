func check(_ condition: @autoclosure () -> Bool, _ message: String) {
    guard condition() else {
        fputs("FAIL: \(message)\n", stderr)
        exit(1)
    }
    print("PASS: \(message)")
}

class AccountCheckDelegate: AppDelegate {
    var requests: [(String, [String: Any])] = []
    var popupOpenedAt: TimeInterval?
    var fixture: [String: Any] = [
        "ready": true, "connected": true, "username": "first",
        "codexEnabled": false, "claudeEnabled": true, "copilotEnabled": true,
        "accounts": [["username": "first", "selected": true], ["username": "second", "selected": false]]
    ]

    override func applicationDidFinishLaunching(_ notification: Notification) {
        check(createWindow(), "The isolated account window loads without a bridge")
        status = fixture
        currentView = "connected"
        render()
        later { self.checkMenu() }
    }

    override func request(_ path: String, body: [String: Any]? = nil,
                          completion: @escaping ([String: Any]?, String?) -> Void) {
        if let body { requests.append((path, body)) }
        var result: [String: Any] = ["ok": true]
        switch path {
        case "/api/status": result = fixture
        case "/api/settings": result = ["closeToTray": true]
        case "/api/local/models": result = ["supported": true, "recommended": "qwen3-4b", "models": [
            ["id": "qwen3-4b", "name": "Qwen3 4B", "bytes": 2500000000, "recommended": true, "available": true],
            ["id": "qwen3-8b", "name": "Qwen3 8B", "bytes": 5000000000, "available": true],
            ["id": "qwen3-coder-30b", "name": "Qwen3-Coder 30B", "available": false, "reason": "Not enough available memory"]
        ]]
        case "/api/local/add": fixture["local"] = ["model": body?["model"] ?? "", "stage": "Downloading", "downloaded": 500000000, "total": 2500000000]
        case "/api/local/cancel": fixture["local"] = ["stage": "Paused"]
        case "/api/accounts/switch":
            let username = body?["username"] as? String ?? ""
            fixture["username"] = username
            fixture["accounts"] = (fixture["accounts"] as! [[String: Any]]).map {
                ["username": $0["username"]!, "selected": $0["username"] as? String == username]
            }
        case "/api/accounts/delete":
            let username = body?["username"] as? String ?? ""
            fixture["accounts"] = (fixture["accounts"] as! [[String: Any]]).filter {
                $0["username"] as? String != username
            }
            if fixture["username"] as? String == username {
                fixture["connected"] = false
                fixture["username"] = ""
            }
            for key in ["connected", "username", "accounts"] { result[key] = fixture[key] }
        default: check(false, "Unexpected mock request \(path)")
        }
        DispatchQueue.main.async { completion(result, nil) }
    }

    override func showError(_ message: String) { check(false, message) }
    override func showAccounts() {
        super.showAccounts()
        // The script dismisses popovers explicitly. External app focus changes
        // must not close a test popup between its scheduled mouse/key actions.
        accountPopover?.behavior = .applicationDefined
        if accountPopover?.isShown == true { popupOpenedAt = ProcessInfo.processInfo.systemUptime }
    }
    func later(_ body: @escaping () -> Void) {
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.15, execute: body)
    }
    func rows() -> [AccountRow] {
        guard let content = accountPopover?.contentViewController?.view,
              let scroll = content.subviews.first as? NSScrollView else {
            check(false, "The account list is in a native scroll view")
            return []
        }
        return scroll.documentView!.subviews.compactMap { $0 as? AccountRow }
    }
    func checkMenu() {
        busy = true
        pendingBridge = "codex"
        render()
        let main = controls["disconnect"]!
        let arrow = controls["accounts"]!
        check(main.frame.maxX == arrow.frame.minX && arrow.frame.width == 28 &&
              arrow.frame.maxX == 349, "Connected has adjacent primary and dropdown hit targets")
        check(window.contentView?.bounds.size == windowSize &&
              controls["claude"]?.frame.minY == 184 && controls["project"]?.frame.minY == footerY,
              "The split button keeps the cards and footer fixed")
        let location = arrow.convert(NSPoint(x: 14, y: 14), to: nil)
        check(window.contentView?.hitTest(location) === arrow, "The arrow receives its own mouse clicks")
        let started = ProcessInfo.processInfo.systemUptime
        for (type, delta) in [(NSEvent.EventType.leftMouseDown, 0.0), (.leftMouseUp, 0.01)] {
            let event = NSEvent.mouseEvent(with: type, location: location, modifierFlags: [],
                                          timestamp: started + delta, windowNumber: window.windowNumber,
                                          context: nil, eventNumber: 1, clickCount: 1, pressure: type == .leftMouseDown ? 1 : 0)!
            NSApp.postEvent(event, atStart: false)
        }
        later {
            check(self.popupOpenedAt != nil && self.popupOpenedAt! - started < 0.2 && self.requests.isEmpty,
                  "A real arrow click opens the menu within 200 ms even while a bridge action is pending (elapsed: \((self.popupOpenedAt ?? started) - started)s)")
            let rows = self.rows()
            check(rows.count == 2 && rows[0].selected && !rows[1].selected,
                  "The dropdown includes the selected account and other saved accounts")
            check(rows[0].choose.accessibilityValue() as? String == "Selected" &&
                  rows[1].remove.accessibilityLabel() == "Remove @second",
                  "Selection and removal have accessible names and values")
            check(rows[0].choose.frame.maxX == rows[0].remove.frame.minX &&
                  rows[0].remove.frame.width >= 24,
                  "Switch and remove have separate non-overlapping hit targets")
            let content = self.accountPopover!.contentViewController!.view
            let divider = content.subviews.compactMap { $0 as? NSBox }.first!
            let add = content.subviews.compactMap { $0 as? NSButton }.first!
            check(!rows[0].choose.isEnabled && !rows[0].remove.isEnabled && !add.isEnabled,
                  "Pending bridge work permits browsing but keeps account mutations disabled")
            self.accountAction("/api/accounts/delete", "first")
            self.addAccount()
            check(self.requests.isEmpty && !self.addingAccount && self.accountPopover?.isShown == true,
                  "Busy account actions cannot switch, delete, or leave the current operation")
            self.busy = false
            self.pendingBridge = nil
            self.render()
            check(rows[0].choose.isEnabled && rows[0].remove.isEnabled && add.isEnabled,
                  "The open account list becomes actionable as soon as bridge work finishes")
            check(add.title == "Add account" && divider.frame.maxY < add.frame.minY,
                  "Add account is last with a divider above it")
            content.window?.makeFirstResponder(add)
            check(!rows[1].highlighted, "The remove glyph is hidden on an idle row")
            rows[1].mouseEntered(with: NSEvent())
            check(rows[1].highlighted, "Hover reveals the row's remove glyph")
            rows[1].mouseExited(with: NSEvent())
            content.window?.makeFirstResponder(rows[1].remove)
            check(rows[1].highlighted, "Keyboard focus also reveals the remove glyph")
            rows[1].choose.performClick(nil)
            self.later {
                check(self.requests.count == 1 && self.requests[0].0 == "/api/accounts/switch" &&
                      self.requests[0].1["username"] as? String == "second",
                      "Clicking the account row sends exactly one switch action")
                check(self.status["username"] as? String == "second" && self.notice.isEmpty &&
                      self.status["copilotEnabled"] as? Bool == true,
                      "The switch shows the confirmed account silently and preserves client preferences")
                self.checkRemove()
            }
        }
    }
    func checkRemove() {
        showAccounts()
        later {
            self.rows()[0].remove.performClick(nil)
            self.later {
                check(self.requests.count == 2 && self.requests.last!.0 == "/api/accounts/delete" &&
                      self.status["username"] as? String == "second",
                      "The X only deletes its account and never switches")
                self.showAccounts()
                self.later {
                    let content = self.accountPopover!.contentViewController!.view
                    content.subviews.compactMap { $0 as? NSButton }.first!.performClick(nil)
                    self.refresh()
                    self.later {
                        check(self.addingAccount && self.currentView == "add-account" &&
                              self.status["connected"] as? Bool == true && self.controls["cancelAccount"]?.isHidden == false,
                              "Add account keeps the live connection and survives status polling")
                        let picker = self.controls["localModel"] as! NSPopUpButton
                        check(self.selectedLocalModel == "qwen3-4b" && picker.selectedItem?.title.contains("Recommended") == true,
                              "Add account selects the recommended local model automatically")
                        check(picker.itemArray.count == 3 && !picker.itemArray[2].isEnabled && self.controls["addGitHub"]?.isHidden == false,
                              "GitHub and local models share the add flow; models that do not fit are disabled")
                        picker.selectItem(at: 1)
                        self.selectLocalModel(picker)
                        self.addLocalModel()
                        self.later {
                            check(self.requests.last!.0 == "/api/local/add" && self.requests.last!.1["model"] as? String == "qwen3-8b",
                                  "Adding a model starts setup with only its identifier")
                            check(self.localSetupPending && self.currentView == "add-account" && self.status["connected"] as? Bool == true,
                                  "Local download preserves the current connection")
                            check(!(self.controls["addLocal"] as! NSButton).isEnabled && self.controls["localProgress"]?.isHidden == false,
                                  "Setup displays progress and prevents duplicate installation")
                            self.cancelAddAccount()
                            self.later { self.checkCancelAndContinue() }
                        }
                    }
                }
            }
        }
    }
    func checkCancelAndContinue() {
                        check(requests.last!.0 == "/api/local/cancel" && !localSetupPending, "Cancel stops the pending setup")
                        self.tokenView()
                        (self.controls["token"] as! NSTextField).stringValue = "temporary-input"
                        self.cancelAddAccount()
                        check(self.currentView == "connected" && !self.addingAccount &&
                              (self.controls["token"] as! NSTextField).stringValue.isEmpty &&
                              self.window.firstResponder === self.controls["accounts"],
                              "Cancel clears token input and returns focus to the split button")
                        self.checkLongList()
    }
    func checkLongList() {
        status["accounts"] = (0..<12).map { ["username": "account-\($0)", "selected": $0 == 0] as [String: Any] }
        showAccounts()
        later {
            check(self.accountPopover!.contentSize.height == 265 && self.rows().count == 12,
                  "Long account lists scroll without resizing the app or losing accounts")
            let rows = self.rows()
            rows[10].moveFocus(1)
            check(rows[11].window?.firstResponder === rows[11].choose &&
                  rows[11].visibleRect.height > 0,
                  "Arrow navigation scrolls the focused account into view")
            let popupWindow = rows[11].window!
            let escape = NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: [],
                                          timestamp: ProcessInfo.processInfo.systemUptime,
                                          windowNumber: popupWindow.windowNumber, context: nil,
                                          characters: "\u{1b}", charactersIgnoringModifiers: "\u{1b}",
                                          isARepeat: false, keyCode: 53)!
            popupWindow.sendEvent(escape)
            self.later {
                check(self.accountPopover == nil && self.window.firstResponder === self.controls["accounts"],
                      "Escape closes the popup and restores focus to its trigger (shown: \(self.accountPopover?.isShown ?? false), focus: \(String(describing: self.window.firstResponder)))")
                self.status = self.fixture
                self.showAccounts()
                self.later {
                    self.rows()[0].remove.performClick(nil)
                    self.later {
                        check(self.currentView == "idle" && self.status["connected"] as? Bool == false &&
                              self.window.firstResponder === self.controls["connect"],
                              "Deleting the last account displays sign-in and restores usable keyboard focus")
                        print("PASS: Account-switcher interactions complete without accessing real credentials or bridge ports")
                        exit(0)
                    }
                }
            }
        }
    }
}

let application = NSApplication.shared
setbuf(stdout, nil)
application.setActivationPolicy(.regular)
let accounts = AccountCheckDelegate()
application.delegate = accounts
DispatchQueue.main.asyncAfter(deadline: .now() + 30) { check(false, "Account checks finish before timeout") }
application.run()

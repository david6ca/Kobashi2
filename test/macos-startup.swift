func check(_ condition: @autoclosure () -> Bool, _ message: String) {
    guard condition() else {
        fputs("FAIL: \(message)\n", stderr)
        exit(1)
    }
    print("PASS: \(message)")
}

class StartupProtocol: URLProtocol {
    enum Mode { case waiting, connected, signedOut, refused, wrongServer, unauthorized, malformed, backendError, settingsError }
    static let lock = NSLock()
    static var mode = Mode.waiting
    static var initialRefusals = 1
    static var statusRequests = 0

    static func configure(_ value: Mode) {
        lock.lock()
        mode = value
        lock.unlock()
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func stopLoading() {}

    override func startLoading() {
        let isStatus = request.url!.path == "/api/status"
        Self.lock.lock()
        let mode = Self.mode
        let refuse = isStatus && (Self.initialRefusals > 0 || mode == .refused)
        if isStatus {
            Self.statusRequests += 1
            if Self.initialRefusals > 0 { Self.initialRefusals -= 1 }
        }
        Self.lock.unlock()
        if refuse {
            client?.urlProtocol(self, didFailWithError: URLError(.cannotConnectToHost))
            return
        }
        var payload: [String: Any] = isStatus
            ? ["ready": mode != .waiting, "connected": mode != .signedOut,
               "username": "example", "version": "2.1.7", "copilotEnabled": false]
            : ["closeToTray": true, "startAtLogin": false]
        var code = 200
        if isStatus, mode == .backendError { payload["error"] = "Client configuration failed." }
        if isStatus, mode == .unauthorized { code = 401; payload = ["error": "Control authentication failed."] }
        if !isStatus, mode == .settingsError { code = 500; payload = ["error": "Settings unavailable."] }
        let headers = mode == .wrongServer && isStatus ? [:] : ["X-Kobashi-Native": "1"]
        let response = HTTPURLResponse(url: request.url!, statusCode: code, httpVersion: "HTTP/1.1", headerFields: headers)!
        let data = mode == .malformed && isStatus
            ? Data("not JSON".utf8) : try! JSONSerialization.data(withJSONObject: payload)
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: data)
        client?.urlProtocolDidFinishLoading(self)
    }
}

class StartupCheckDelegate: AppDelegate {
    override func applicationDidFinishLaunching(_ notification: Notification) {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/cat")
        let input = Pipe()
        process.standardInput = input
        process.standardOutput = FileHandle.nullDevice
        do { try process.run() } catch { check(false, "The disposable child starts: \(error)") }
        serverProcess = process
        serverInput = input
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [StartupProtocol.self]
        localSession = URLSession(configuration: configuration)
        check(createWindow(), "The isolated startup window loads")
        if let expected = ProcessInfo.processInfo.environment["KOBASHI_TEST_SCREEN_ID"] {
            let display = window.screen?.deviceDescription[NSDeviceDescriptionKey("NSScreenNumber")] as? NSNumber
            check(display?.stringValue == expected, "The test window stays on the requested display")
        }
        check(currentView == "starting" && (controls["connect"] as? NSButton)?.title == "Starting..." &&
              (controls["connect"] as? NSButton)?.accessibilityLabel() == "Starting..." &&
              (controls["connect"] as? NSButton)?.isEnabled == false &&
              controls["tokenView"]?.isHidden == true,
              "Loading uses a disabled, accessible status instead of flashing Not Connected")
        connect()
        check(!busy && StartupProtocol.statusRequests == 0, "Sign-in cannot race saved-login initialization")
        poll(.waiting) {
            check(self.currentView == "starting" && !self.bridgeReady && !self.startupHandled &&
                  self.notice.isEmpty && self.refreshError.isEmpty && StartupProtocol.statusRequests == 2,
                  "An initial connection refusal retries silently and incomplete initialization keeps waiting")
            self.poll(.connected) {
                check(self.currentView == "connected" && self.bridgeReady && self.startupHandled &&
                      self.controls["connect"]?.isHidden == true && self.controls["claude"]?.isHidden == false,
                      "Restored login opens the connected screen only after initialization")
                check((self.controls["copilot"] as? NativeButton)?.bridgeStatusText == "Paused",
                      "A disabled Copilot uses the same Paused text as Codex")
                self.poll(.signedOut) {
                    check(self.currentView == "idle" && (self.controls["connect"] as? NSButton)?.title == "Not Connected" &&
                          (self.controls["connect"] as? NSButton)?.isEnabled == true,
                          "A genuinely signed-out account offers sign-in once ready")
                    self.checkFailures()
                }
            }
        }
    }

    func poll(_ mode: StartupProtocol.Mode, _ completion: @escaping () -> Void) {
        StartupProtocol.configure(mode)
        refresh()
        awaitRefresh(Date().addingTimeInterval(3), completion)
    }

    func awaitRefresh(_ deadline: Date, _ completion: @escaping () -> Void) {
        if Date() >= deadline { check(false, "The isolated refresh stays bounded") }
        if refreshing {
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.01) { self.awaitRefresh(deadline, completion) }
        } else {
            completion()
        }
    }

    func checkFailures() {
        bridgeReady = false
        currentView = "starting"
        startupDeadline = Date.distantPast
        poll(.refused) {
            check(self.refreshError.contains("Kobashi2's local bridge") && self.notice.isEmpty &&
                  (self.controls["connect"] as? NSButton)?.title == "Unavailable",
                  "An expired startup retry displays the actual local-bridge failure")
            self.poll(.connected) {
                check(self.refreshError.isEmpty && self.notice.isEmpty && self.controls["notice"]?.isHidden == true,
                      "A successful refresh clears an old connection error even after settings initialized")
                self.notice = "Could not enable bridge."
                self.poll(.connected) {
                    check(self.notice == "Could not enable bridge." &&
                          (self.controls["notice"] as? NSTextField)?.stringValue == self.notice,
                          "Polling success never clears an actionable operation error")
                    self.notice = ""
                    self.poll(.backendError) {
                        check((self.controls["notice"] as? NSTextField)?.stringValue == "Client configuration failed.",
                              "Real backend initialization errors stay visible")
                        self.poll(.settingsError) {
                            check(self.refreshError == "Settings unavailable.", "Settings polling errors remain explicit")
                            self.poll(.connected) {
                                check(self.refreshError.isEmpty && self.controls["notice"]?.isHidden == true,
                                      "Recovered settings polling errors clear without a success notice")
                                self.checkInvalidResponses([.wrongServer, .unauthorized, .malformed])
                            }
                        }
                    }
                }
            }
        }
    }

    func checkInvalidResponses(_ modes: [StartupProtocol.Mode]) {
        guard let mode = modes.first else { finish(); return }
        bridgeReady = false
        currentView = "starting"
        startupDeadline = Date().addingTimeInterval(10)
        let count = StartupProtocol.statusRequests
        poll(mode) {
            check(!self.refreshError.isEmpty && StartupProtocol.statusRequests == count + 1,
                  "Port conflicts, authentication failures, and invalid responses are not hidden by startup retries")
            check(self.window.contentView?.bounds.size == NSSize(width: 400, height: 390) &&
                  self.controls["claude"]?.frame.minY == 184 && self.controls["project"]?.frame.minY == footerY,
                  "Startup and error states preserve the fixed window and anchored footer")
            self.checkInvalidResponses(Array(modes.dropFirst()))
        }
    }

    func finish() {
        serverInput?.fileHandleForWriting.closeFile()
        serverProcess?.waitUntilExit()
        poll(.connected) {
            check(self.refreshError == "The native bridge is not running.",
                  "A stopped child is reported immediately rather than retried as startup")
            self.localSession.invalidateAndCancel()
            print("PASS: Native startup, login readiness, error recovery, and Paused status")
            exit(0)
        }
    }
}

let application = NSApplication.shared
application.setActivationPolicy(.regular)
let startup = StartupCheckDelegate()
application.delegate = startup
DispatchQueue.main.asyncAfter(deadline: .now() + 15) { check(false, "The isolated startup checks finish before timeout") }
application.run()

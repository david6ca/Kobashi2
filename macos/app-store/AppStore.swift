import Cocoa
import Darwin
import ServiceManagement

final class AppStoreDelegate: AppDelegate {
    private var scopes: [String: URL] = [:]

    override func action(_ path: String, _ body: [String: Any] = [:]) {
        let clients = ["/api/toggle-codex": "codex", "/api/toggle-claude": "claude", "/api/toggle-copilot": "copilot"]
        if let client = clients[path], status[client + "Enabled"] as? Bool != true,
           !authorizeClientFolder(client) { return }
        super.action(path, body)
    }

    private func authorizeClientFolder(_ client: String) -> Bool {
        if scopes[client] != nil { return true }
        let bookmarks = bookmarkFile()
        var saved = (NSDictionary(contentsOf: bookmarks) as? [String: Data]) ?? [:]
        if let data = saved[client] {
            var stale = false
            if let url = try? URL(resolvingBookmarkData: data, options: [.withSecurityScope, .withoutUI],
                                  relativeTo: nil, bookmarkDataIsStale: &stale),
               !stale, url.startAccessingSecurityScopedResource() {
                scopes[client] = url
                return true
            }
        }
        let panel = NSOpenPanel()
        panel.title = ["codex": "Codex", "claude": "Claude Code", "copilot": "GitHub Copilot"][client]!
        panel.message = L("Choose client configuration folder")
        panel.prompt = L("Allow")
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.showsHiddenFiles = true
        if let user = getpwuid(getuid()), let home = user.pointee.pw_dir {
            panel.directoryURL = URL(fileURLWithPath: String(cString: home)).appendingPathComponent("." + client)
        }
        guard panel.runModal() == .OK, let url = panel.url else { return false }
        do {
            let data = try url.bookmarkData(options: [.withSecurityScope], includingResourceValuesForKeys: nil, relativeTo: nil)
            guard url.startAccessingSecurityScopedResource() else { throw CocoaError(.fileReadNoPermission) }
            saved[client] = data
            try FileManager.default.createDirectory(at: bookmarks.deletingLastPathComponent(), withIntermediateDirectories: true)
            guard (saved as NSDictionary).write(to: bookmarks, atomically: true) else { throw CocoaError(.fileWriteUnknown) }
            scopes[client] = url
            return true
        } catch {
            notice = L("Could not save client access")
            render()
            return false
        }
    }

    private func bookmarkFile() -> URL {
        URL(fileURLWithPath: NSHomeDirectory()).appendingPathComponent(".kobashi/client-access.plist")
    }

    override func request(_ path: String, body: [String: Any]? = nil,
                          completion: @escaping ([String: Any]?, String?) -> Void) {
        guard path == "/api/settings", let enabled = body?["startAtLogin"] as? Bool else {
            super.request(path, body: body, completion: completion)
            return
        }
        let previous = SMAppService.mainApp.status == .enabled
        do {
            if enabled != previous {
                if enabled { try SMAppService.mainApp.register() }
                else { try SMAppService.mainApp.unregister() }
            }
        } catch { completion(nil, error.localizedDescription); return }
        super.request(path, body: body) { value, error in
            if error != nil, enabled != previous {
                if previous { try? SMAppService.mainApp.register() }
                else { try? SMAppService.mainApp.unregister() }
            }
            completion(value, error)
        }
    }

    override func applicationWillTerminate(_ notification: Notification) {
        for url in scopes.values { url.stopAccessingSecurityScopedResource() }
        super.applicationWillTerminate(notification)
    }
}

func runAppStoreSelfTest() throws {
    NSApp.setActivationPolicy(.prohibited)
    let home = URL(fileURLWithPath: NSHomeDirectory()).appendingPathComponent("Library/Caches/StoreCheck-" + UUID().uuidString)
    try FileManager.default.createDirectory(at: home, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: home) }
    let resources = Bundle.main.resourceURL!
    let engine = Process()
    engine.executableURL = resources.appendingPathComponent("Runtime/llama-server")
    engine.arguments = ["--version"]
    var environment = ProcessInfo.processInfo.environment
    environment["HOME"] = home.path
    engine.environment = environment
    try engine.run()
    engine.waitUntilExit()
    guard engine.terminationStatus == 0 else { throw NSError(domain: "StoreCheck", code: Int(engine.terminationStatus)) }
    print("Bundled native llama.cpp starts without Python")

    let process = Process()
    #if arch(arm64)
    process.executableURL = resources.appendingPathComponent("bin/kobashi-arm64")
    #else
    process.executableURL = resources.appendingPathComponent("bin/kobashi-x64")
    #endif
    let token = UUID().uuidString
    environment["KOBASHI_APP_PATH"] = Bundle.main.bundleURL.path
    environment["KOBASHI_CONTROL_TOKEN"] = token
    process.environment = environment
    process.arguments = ["--no-open", "--home", home.path, "--port-offset", "2200", "--no-startup-registration"]
    let input = Pipe()
    process.standardInput = input
    try process.run()
    defer { input.fileHandleForWriting.closeFile(); process.waitUntilExit() }
    let configuration = URLSessionConfiguration.ephemeral
    configuration.connectionProxyDictionary = ["HTTPEnable":0,"HTTPSEnable":0,"SOCKSEnable":0]
    let session = URLSession(configuration: configuration)
    var ready = false
    for _ in 0..<40 {
        let semaphore = DispatchSemaphore(value: 0)
        var request = URLRequest(url: URL(string: "http://127.0.0.1:21122/api/status")!)
        request.timeoutInterval = 1
        request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
        session.dataTask(with: request) { data, _, _ in
            if let data, let value = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                ready = value["ready"] as? Bool == true && value["connected"] as? Bool == false
            }
            semaphore.signal()
        }.resume()
        semaphore.wait()
        if ready { break }
        Thread.sleep(forTimeInterval: 0.1)
    }
    guard ready else { throw NSError(domain: "StoreCheck", code: 1) }
    print("Sandboxed bridge starts on isolated ports without accessing existing accounts")
}

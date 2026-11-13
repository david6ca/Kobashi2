func check(_ condition: @autoclosure () -> Bool, _ message: String) {
    guard condition() else { fputs("FAIL: \(message)\n", stderr); exit(1) }
    print("PASS: \(message)")
}

final class LocalizationCheckDelegate: AppDelegate {
    var requests: [(String, [String: Any])] = []
    var failSave = false
    var browserOpens = 0
    var holdDeviceReply = false
    var deviceReply: (([String: Any]?, String?) -> Void)?
    let deviceFixture: [String: Any] = ["user_code": "ABCD-EFGH", "device_code": "mock-device", "verification_uri": "https://github.com/login/device", "expires_in": 900]
    override func request(_ path: String, body: [String: Any]? = nil,
                          completion: @escaping ([String: Any]?, String?) -> Void) {
        requests.append((path, body ?? [:]))
        if path == "/api/start" {
            if holdDeviceReply { deviceReply = completion } else { completion(deviceFixture, nil) }
            return
        }
        if path == "/api/local/models" {
            completion(["supported": true, "recommended": "fixture-model", "models": [
                ["id": "fixture-model", "name": "Fixture", "available": true, "bytes": 1024]
            ]], nil)
            return
        }
        check(path == "/api/language", "Language changes never call account or inference APIs")
        check(notice.isEmpty, "Language saves stay silent")
        if failSave { completion(nil, "Could not save language.") }
        else { completion(body, nil) }
    }
    override func refresh() {}
    override func openGitHub() { browserOpens += 1 }
    override func showError(_ message: String) {}
}

func capture(_ subject: AppDelegate, directory: URL, name: String, caption: String) throws {
    let content = subject.window.contentView!
    let rep = content.bitmapImageRepForCachingDisplay(in: content.bounds)!
    content.cacheDisplay(in: content.bounds, to: rep)
    let native = NSImage(size: content.bounds.size)
    native.addRepresentation(rep)
    let context = CGContext(data: nil, width: 2560, height: 1600, bitsPerComponent: 8, bytesPerRow: 0,
                            space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)!
    let previous = NSGraphicsContext.current
    NSGraphicsContext.current = NSGraphicsContext(cgContext: context, flipped: false)
    NativeColors.rgb(0xfff5f3).setFill()
    NSRect(x: 0, y: 0, width: 2560, height: 1600).fill()
    let paragraph = NSMutableParagraphStyle()
    paragraph.alignment = .center
    paragraph.baseWritingDirection = L10n.isRTL ? .rightToLeft : .natural
    (caption as NSString).draw(in: NSRect(x: 100, y: 1430, width: 2360, height: 90),
                              withAttributes: [.font: NSFont.systemFont(ofSize: 56, weight: .semibold),
                                               .foregroundColor: NativeColors.rgb(0x3d202a), .paragraphStyle: paragraph])
    native.draw(in: NSRect(x: 680, y: 170, width: 1200, height: 1170), from: .zero, operation: .sourceOver, fraction: 1)
    NSGraphicsContext.current = previous
    let canvas = NSBitmapImageRep(cgImage: context.makeImage()!)
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    try canvas.representation(using: .png, properties: [:])!.write(to: directory.appendingPathComponent(name + ".png"))
}

let application = NSApplication.shared
application.setActivationPolicy(.prohibited)
let subject = LocalizationCheckDelegate()
check(subject.createWindow(show: false), "Native controls load without opening a window")
subject.status = ["ready": true, "connected": true, "username": "sample-account", "version": "2.2.1",
                  "codexEnabled": true, "claudeEnabled": true, "copilotEnabled": false,
                  "accounts": [["username": "sample-account", "selected": true]],
                  "usage": ["creditsUsed": 12.5, "entitlement": 100]]
let originalSize = homeWindowSize
for language in L10n.languages {
    let code = language["code"]!
    subject.status["language"] = code
    subject.currentView = "connected"
    subject.render()
    check(L10n.current == code, "\(code) applies immediately")
    check((subject.controls["codex"] as! NSButton).state == .on &&
          subject.status["username"] as? String == "sample-account", "\(code) preserves accounts and toggles")
    let x = subject.controls["settings"]!.frame.minX
    subject.render()
    check(subject.controls["settings"]!.frame.minX == x, "\(code) repeated renders do not flip layout twice")
    check(subject.window.contentView!.bounds.size == originalSize && !subject.window.isVisible, "\(code) preserves the fixed hidden window")
    check(L10n.privacy.contains("support@pomegram.com") && !L10n.privacy.contains("privacy."), "\(code) includes the offline privacy statement")
    let menu = NSApp.mainMenu!.items[0].submenu!
    let languages = menu.items.first { $0.title == L("Language") }!.submenu!
    check(languages.items.count == 14 && languages.items.filter { $0.state == .on }.count == 1,
          "\(code) menu offers Auto plus 13 locales with one saved choice")
    check(menu.items.contains { $0.title == L("Privacy Statement") && $0.action == #selector(AppDelegate.showPrivacyStatement) }, "\(code) menu opens privacy")
    check(directionalPoint(NSPoint(x: 312, y: 16), width: 336).x == (code == "ar" ? 24 : 312), "\(code) checkbox paths follow their mirrored control bounds")
    check(subject.window.toolbar?.items.contains { $0.itemIdentifier.rawValue == "settings" } == true, "\(code) keeps Settings in native window chrome without mirroring window controls")
    let privacy = subject.preparePrivacyStatement()
    check(!privacy.isVisible && subject.privacyText?.string == L10n.privacy && privacy.title == L("Privacy Statement"), "\(code) privacy opens a readable native document offline")
    if let output = ProcessInfo.processInfo.environment["KOBASHI_SCREENSHOT_OUTPUT"],
       ProcessInfo.processInfo.environment["KOBASHI_SCREENSHOT_LOCALE"] == nil || ProcessInfo.processInfo.environment["KOBASHI_SCREENSHOT_LOCALE"] == code {
        let folder = URL(fileURLWithPath: output).appendingPathComponent(code)
        try capture(subject, directory: folder, name: "01-clients", caption: L("One source. Your coding tools."))
        subject.currentView = "settings"
        subject.render()
        try capture(subject, directory: folder, name: "02-settings", caption: L("Your language. Your workflow."))
    }
    subject.currentView = "settings"
    subject.render()
    let selector = subject.controls["language"] as! NSPopUpButton
    check(selector.numberOfItems == 14 && selector.selectedItem?.representedObject as? String == code,
          "\(code) Settings offers Auto plus 13 locales and selects the saved choice")
    check(subject.controls["privacy"] == nil, "\(code) Privacy is available only through the system menu")
    check(subject.controls["title"] == nil && subject.controls["project"] == nil && subject.controls["settings"]!.isHidden && !subject.controls["settingsBack"]!.isHidden,
          "\(code) Settings replaces branding and its icon with Back")
    check(subject.makeStatusMenu().items.filter { !$0.isSeparatorItem }.map(\.title) == [L("Show ModelBridge"), L("Quit ModelBridge")],
          "\(code) tray menu contains only Show and Quit")
    check(subject.controls["done"] == nil &&
          settingsWindowSize.height - subject.controls["language"]!.frame.maxY == 24,
          "\(code) Settings uses Back and has no Done footer")
    let beforeGitHub = subject.requests.count
    subject.addGitHubAccount()
    check(subject.currentView == "device" && subject.requests.count == beforeGitHub + 1 &&
          subject.requests.last?.0 == "/api/start" && subject.browserOpens == 0 &&
          subject.controls["localModel"]!.isHidden && subject.controls["token"] == nil && subject.controls["tokenView"] == nil && subject.controls["login"] == nil &&
          !subject.controls["code"]!.isHidden && subject.controls["addGitHub"] == nil &&
          (subject.controls["githubDeviceTitle"] as! NSTextField).stringValue == L("Add GitHub account"),
          "\(code) GitHub only offers device authorization, without token input or auto-opening a browser")
    check(subject.window.contentView!.bounds.size == githubDeviceWindowSize &&
          subject.controls["githubDeviceTitle"]!.frame.minY == 8 &&
          subject.controls["cancelAccount"]!.frame.minY == 0 &&
          subject.controls["browser"]!.frame.minX == 40 && subject.controls["browser"]!.frame.maxX == 360 &&
          subject.controls["copy"]!.frame.maxY == 148,
          "\(code) Unified GitHub setup retains the header and 24-point page margins")
    subject.cancelAddAccount()
    subject.addLocalAccount()
    check(subject.currentView == "add-account" && subject.requests.last?.0 == "/api/local/models" &&
          subject.controls["token"] == nil && subject.controls["code"]!.isHidden &&
          (subject.controls["addAccountTitle"] as! NSTextField).stringValue == L("Add a local model"),
          "\(code) Local model setup remains separate from GitHub auth")
    let add = subject.controls["addLocal"] as! NativeButton
    check(add.frame.minX == 24 && add.frame.maxX == 376 &&
          addAccountWindowSize.height - add.frame.maxY == 24 && add.isPrimaryAction && add.isEnabled &&
          subject.window.defaultButtonCell === add.cell,
          "\(code) Local Add retains the 24-point footer margin and Enter behavior")
    if ProcessInfo.processInfo.environment["KOBASHI_SCREENSHOT_OUTPUT"] != nil, ["en", "ar"].contains(code) {
        try capture(subject, directory: URL(fileURLWithPath: "/private/tmp/modelbridge-ui-review/" + code), name: "add-account", caption: L("One source. Your coding tools."))
    }
    subject.addingAccount = false
    subject.currentView = "settings"
    subject.render()
    check((subject.controls["settingsTitle"] as! NSTextField).stringValue == L("Settings"), "\(code) settings and accessibility labels are translated")
}
check(L10n.resolve("auto", preferred: ["xx", "de-AT", "zh-TW"]) == "de" &&
      L10n.resolve("auto", preferred: ["zh-Hans-HK"]) == "zh-Hans" &&
      L10n.resolve("auto", preferred: ["zh-HK"]) == "zh-Hant" &&
      L10n.resolve("auto", preferred: ["pt-PT"]) == "pt-BR", "System preferences retain language and script priority")
subject.status["language"] = "en"
subject.render()
let choose = subject.controls["language"] as! NSPopUpButton
choose.selectItem(at: choose.itemArray.firstIndex { $0.representedObject as? String == "de" }!)
subject.changeSettingsLanguage(choose)
check(subject.status["language"] as? String == "de" && L10n.current == "de", "Settings applies the confirmed saved language")
subject.failSave = true
choose.selectItem(at: choose.itemArray.firstIndex { $0.representedObject as? String == "ar" }!)
subject.changeSettingsLanguage(choose)
check(subject.status["language"] as? String == "de" && L10n.current == "de" && choose.selectedItem?.representedObject as? String == "de" && !subject.notice.isEmpty, "Failed saves restore the saved Settings choice and show an error")
subject.notice = ""
subject.done()
check(subject.currentView == "connected" && subject.controls["settingsBack"]!.isHidden && !subject.controls["settings"]!.isHidden,
      "Settings Back returns to the unchanged connected screen")
check(L10n.format("{0} tokens", "private {1}") == "private {1} Tokens", "Inserted data is neither translated nor interpolated again")
check(subject.serverProcess == nil && !subject.window.isVisible, "Checks never start a bridge or show a desktop window")
print("PASS: 13 locales, menu actions, RTL, privacy and optional store screenshot capture")

subject.notice = ""
subject.status["language"] = "en"
subject.currentView = "connected"
subject.addingAccount = false
subject.holdDeviceReply = true
subject.addGitHubAccount()
check(subject.deviceLoading && (subject.controls["cancelAccount"] as! NSButton).isEnabled &&
      subject.controls["token"] == nil,
      "Device authorization can be cancelled while code generation is pending, with no token entry")
subject.cancelAddAccount()
let cancelledView = subject.currentView
subject.deviceReply?(subject.deviceFixture, nil)
check(subject.currentView == cancelledView && !subject.addingAccount && subject.deviceCode.isEmpty && !subject.deviceLoading,
      "A late device-code response cannot reopen a cancelled GitHub flow")

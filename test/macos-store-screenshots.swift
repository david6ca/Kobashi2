
final class StoreCaptureDelegate: AppDelegate {
    override func refresh() {}
    override func request(_ path: String, body: [String: Any]? = nil,
                          completion: @escaping ([String: Any]?, String?) -> Void) {
        completion(nil, "Screenshot preview has no bridge")
    }
    override func showError(_ message: String) { fputs(message + "\n", stderr) }
}

@available(macOS 14.0, *)
@MainActor func captureStoreWindow(_ subject: StoreCaptureDelegate, code: String, name: String, caption: String, output: URL) async throws {
    subject.window.ignoresMouseEvents = true
    subject.window.collectionBehavior.insert(.moveToActiveSpace)
    subject.window.makeKeyAndOrderFront(nil)
    NSApp.activate(ignoringOtherApps: true)
    subject.window.displayIfNeeded()
    try await Task.sleep(nanoseconds: 250_000_000)
    let raw = URL(fileURLWithPath: ProcessInfo.processInfo.environment["KOBASHI_SCREENSHOT_RAWDIR"]!).appendingPathComponent("native-" + code + "-" + name + ".png")
    try FileManager.default.createDirectory(at: output, withIntermediateDirectories: true)
    let request = output.appendingPathComponent(".capture-request.json")
    let data = try JSONSerialization.data(withJSONObject: ["windowID":subject.window.windowNumber,"path":raw.path,"visible":subject.window.isVisible,"screenCaptureAccess":CGPreflightScreenCaptureAccess()])
    try data.write(to: request, options: .atomic)
    let deadline = Date().addingTimeInterval(30)
    while !FileManager.default.fileExists(atPath: raw.path) && Date() < deadline {
        try await Task.sleep(nanoseconds: 100_000_000)
    }
    let bitmap = NSBitmapImageRep(data: try Data(contentsOf: raw))!
    let captured = bitmap.cgImage!
    defer { try? FileManager.default.removeItem(at: raw) }
    let native = NSImage(cgImage: captured, size: NSSize(width: captured.width, height: captured.height))
    let canvas = CGContext(data: nil, width: 2560, height: 1600, bitsPerComponent: 8, bytesPerRow: 0,
                           space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)!
    let previous = NSGraphicsContext.current
    NSGraphicsContext.current = NSGraphicsContext(cgContext: canvas, flipped: false)
    NativeColors.rgb(0xfff5f3).setFill()
    NSRect(x: 0, y: 0, width: 2560, height: 1600).fill()
    let paragraph = NSMutableParagraphStyle()
    paragraph.alignment = .center
    paragraph.baseWritingDirection = code == "ar" ? .rightToLeft : .natural
    (caption as NSString).draw(in: NSRect(x: 100, y: 1405, width: 2360, height: 100), withAttributes: [
        .font: NSFont.systemFont(ofSize: 56, weight: .semibold),
        .foregroundColor: NativeColors.rgb(0x3d202a), .paragraphStyle: paragraph])
    let scale = min(1600 / CGFloat(captured.width), 1050 / CGFloat(captured.height))
    let width = CGFloat(captured.width) * scale, height = CGFloat(captured.height) * scale
    native.draw(in: NSRect(x: (2560-width)/2, y: 180+(1050-height)/2, width: width, height: height))
    NSGraphicsContext.current = previous
    let directory = output.appendingPathComponent(code)
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    let pixels = NSBitmapImageRep(cgImage: canvas.makeImage()!)
    try pixels.representation(using: .png, properties: [:])!.write(to: directory.appendingPathComponent(name + ".png"))
    print("Captured \(code)/\(name): native \(captured.width)x\(captured.height), preserved aspect ratio")
}

let application = NSApplication.shared
application.setActivationPolicy(.regular)
let subject = StoreCaptureDelegate()
guard subject.createWindow(show: false) else { exit(1) }
subject.window.setFrameOrigin(NSPoint(x: NSScreen.main!.visibleFrame.minX + 80, y: NSScreen.main!.visibleFrame.minY + 100))
let output = URL(fileURLWithPath: ProcessInfo.processInfo.environment["KOBASHI_SCREENSHOT_OUTPUT"]!)
subject.status = ["ready":true,"connected":true,"username":"sample-account","version":"2.2.1",
                  "codexEnabled":true,"claudeEnabled":true,"copilotEnabled":false,
                  "accounts":[["username":"sample-account","selected":true]],
                  "usage":["creditsUsed":12.5,"entitlement":100]]
Task { @MainActor in
    do {
        guard #available(macOS 14.0, *) else { throw NSError(domain:"StoreCapture",code:2) }
        for language in L10n.languages {
            let code = language["code"]!
            if let requested = ProcessInfo.processInfo.environment["KOBASHI_SCREENSHOT_LOCALE"], requested != code { continue }
            subject.status["language"] = code
            subject.currentView = "connected"; subject.render()
            try await captureStoreWindow(subject, code:code, name:"01-clients", caption:L("One source. Your coding tools."), output:output)
            subject.currentView = "settings"; subject.render()
            try await captureStoreWindow(subject, code:code, name:"02-settings", caption:L("Your language. Your workflow."), output:output)
        }
        subject.window.orderOut(nil)
        exit(0)
    } catch { fputs("Native store capture failed: \(error)\n", stderr); exit(1) }
}
application.run()

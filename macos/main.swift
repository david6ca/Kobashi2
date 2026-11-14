import Cocoa

enum L10n {
    static var selection = "auto"
    static var preferences: [String] { Locale.preferredLanguages }
    static let catalog: [String: Any] = {
        guard let url = Bundle.main.url(forResource: "catalog", withExtension: "json"),
              let data = try? Data(contentsOf: url),
              let value = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            fatalError("Missing ModelBridge localization catalog")
        }
        return value
    }()
    static var languages: [[String: String]] { catalog["languages"] as! [[String: String]] }
    static func resolve(_ selection: String, preferred: [String]) -> String {
        let supported = languages.compactMap { $0["code"] }
        if supported.contains(selection) { return selection }
        for preference in preferred {
            let parts = preference.replacingOccurrences(of: "_", with: "-").lowercased().split(separator: "-").map(String.init)
            guard let language = parts.first else { continue }
            if language == "zh" {
                if parts.contains("hant") { return "zh-Hant" }
                if parts.contains("hans") { return "zh-Hans" }
                return parts.contains(where: { ["tw", "hk", "mo"].contains($0) }) ? "zh-Hant" : "zh-Hans"
            }
            if language == "pt" { return "pt-BR" }
            if supported.contains(language) { return language }
        }
        return "en"
    }
    static var current: String { resolve(selection, preferred: preferences) }
    static var locale: Locale { Locale(identifier: current) }
    static var isRTL: Bool { current == "ar" }
    static func text(_ source: String) -> String {
        let values = (catalog["messages"] as? [String: [String: String]])?[source]
        return values?[current] ?? values?["en"] ?? source
    }
    static func format(_ source: String, _ arguments: String...) -> String {
        let template = text(source)
        // Replace placeholders in the template, never inside inserted user data.
        let pattern = try! NSRegularExpression(pattern: #"\{([0-9]+)\}"#)
        let result = NSMutableString(string: template)
        for match in pattern.matches(in: template, range: NSRange(template.startIndex..., in: template)).reversed() {
            let number = (template as NSString).substring(with: match.range(at: 1))
            if let index = Int(number), arguments.indices.contains(index) {
                result.replaceCharacters(in: match.range, with: arguments[index])
            }
        }
        return result as String
    }
    static var privacy: String { (0..<6).map { text("privacy.\($0)") }.joined(separator: "\n\n") }
}

func L(_ source: String) -> String { L10n.text(source) }

func directionalRect(_ rect: NSRect, width: CGFloat) -> NSRect {
    L10n.isRTL ? NSRect(x: width - rect.maxX, y: rect.minY, width: rect.width, height: rect.height) : rect
}


let UI_PORT = 18922
let windowSize = NSSize(width: 400, height: 350)
let homeWindowSize = NSSize(width: 400, height: 259)
let settingsWindowSize = NSSize(width: 400, height: 220)
let addAccountWindowSize = NSSize(width: 400, height: 212)
let githubDeviceWindowSize = NSSize(width: 400, height: 232)
let footerY: CGFloat = 356
let mainGlassY: CGFloat = 91
let localContextSliderFrame = NSRect(x: 28, y: 64, width: 344, height: 24)
let localContextBottomMargin: CGFloat = 20
let homeMargin: CGFloat = 28
let windowControlTop: CGFloat = 20
let settingsGlyphTop: CGFloat = 20
let settingsGlyphRight: CGFloat = 24

struct NativeColors {
    let background, text, muted, panel, border, button, hover, input: NSColor
    let accent, secondary, danger, track, on, tile, badge: NSColor
    let connected, disconnected: NSColor

    static func rgb(_ value: UInt32) -> NSColor {
        NSColor(srgbRed: CGFloat((value >> 16) & 255) / 255,
                green: CGFloat((value >> 8) & 255) / 255,
                blue: CGFloat(value & 255) / 255, alpha: 1)
    }

    static let brandPink = rgb(0xe94f76)
    static let brandOrange = rgb(0xff9166)

    static func appStoreWhite(_ ordinary: NSColor) -> NSColor {
        #if APP_STORE
        return .white
        #else
        return ordinary
        #endif
    }

    static var usesGlass: Bool {
        !NSWorkspace.shared.accessibilityDisplayShouldReduceTransparency &&
        !NSWorkspace.shared.accessibilityDisplayShouldIncreaseContrast
    }

    static var gradient: NSGradient {
        let high = NSWorkspace.shared.accessibilityDisplayShouldIncreaseContrast
        return NSGradient(starting: high ? current.accent : brandPink,
                          ending: high ? current.secondary : brandOrange)!
    }

    static var current: NativeColors {
        let dark = NSApp.effectiveAppearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
        return palette(dark: dark)
    }

    static var onBlack: NativeColors { palette(dark: true) }

    private static func palette(dark: Bool) -> NativeColors {
        let values: [UInt32] = dark
            ? [0x191014, 0xfff0f4, 0xbda2ac, 0x261a20, 0x49303a, 0x302129, 0x402934, 0x20151a,
               0xe94f76, 0xff9166, 0xf85149, 0x8d7179, 0xc9355d, 0x33222a, 0x2c1520]
            : [0xfff5f3, 0x3d202a, 0x80616c, 0xfffcfb, 0xf0d8dd, 0xffffff, 0xffe6e5, 0xfff9f7,
               0xb52d52, 0xb94722, 0xdc2626, 0x9c7783, 0xb52d52, 0xffffff, 0xffe7ed]
        let c = values.map(rgb)
        let high = NSWorkspace.shared.accessibilityDisplayShouldIncreaseContrast
        let glass = usesGlass
        return NativeColors(background: c[0],
                            text: c[1], muted: high ? c[1] : c[2],
                            panel: c[3].withAlphaComponent(glass ? (dark ? 0.42 : 0.52) : 1),
                            border: high ? c[1] : glass ? NSColor.white.withAlphaComponent(dark ? 0.20 : 0.72) : c[4],
                            button: c[5].withAlphaComponent(glass ? 0.48 : 1),
                            hover: c[6].withAlphaComponent(glass ? 0.65 : 1),
                            input: c[7].withAlphaComponent(glass ? 0.48 : 1),
                            accent: c[8], secondary: c[9], danger: c[10], track: c[11],
                            on: c[12], tile: c[13].withAlphaComponent(glass ? 0.36 : 1),
                            badge: c[14].withAlphaComponent(glass ? 0.50 : 1),
                            connected: high ? c[1] : rgb(dark ? 0x3fb950 : 0x15803d),
                            disconnected: high ? c[1] : rgb(dark ? 0x9ca3af : 0x6b7280))
    }
}

func rounded(_ rect: NSRect, radius: CGFloat, fill: NSColor, border: NSColor) {
    let path = NSBezierPath(roundedRect: rect.insetBy(dx: 0.5, dy: 0.5), xRadius: radius, yRadius: radius)
    fill.setFill()
    path.fill()
    if NativeColors.usesGlass && fill.alphaComponent < 1 {
        NSGradient(starting: NSColor.white.withAlphaComponent(0.14),
                   ending: NSColor.white.withAlphaComponent(0.01))?.draw(in: path, angle: 90)
    }
    border.setStroke()
    path.lineWidth = 1
    path.stroke()
}

func menuBarIcon() -> NSImage {
    let image = NSImage(size: NSSize(width: 22, height: 18), flipped: false) { _ in
        NSColor.black.setStroke()
        let connector = NSBezierPath()
        connector.move(to: NSPoint(x: 8.5, y: 9))
        connector.line(to: NSPoint(x: 13.5, y: 9))
        connector.lineWidth = 1.5
        connector.lineCapStyle = .round
        connector.stroke()
        for x: CGFloat in [1.5, 13.5] {
            let circle = NSBezierPath(ovalIn: NSRect(x: x, y: 5.5, width: 7, height: 7))
            circle.lineWidth = 1.5
            circle.stroke()
        }
        return true
    }
    image.isTemplate = true
    return image
}

func directionalPoint(_ point: NSPoint, width: CGFloat) -> NSPoint {
    L10n.isRTL ? NSPoint(x: width - point.x, y: point.y) : point
}

func nativeText(_ text: String, in rect: NSRect, font: NSFont, color: NSColor,
                alignment: NSTextAlignment = .left) {
    let paragraph = NSMutableParagraphStyle()
    paragraph.alignment = L10n.isRTL && alignment == .left ? .right : alignment
    paragraph.baseWritingDirection = L10n.isRTL ? .rightToLeft : .natural
    paragraph.lineBreakMode = .byTruncatingTail
    (text as NSString).draw(in: rect, withAttributes: [.font: font, .foregroundColor: color, .paragraphStyle: paragraph])
}

func gradientText(_ text: String, in rect: NSRect, font: NSFont, centered: Bool = false, spacing: CGFloat = 0) {
    let mask = NSImage(size: rect.size, flipped: true) { box in
        let paragraph = NSMutableParagraphStyle()
        paragraph.alignment = centered ? .center : L10n.isRTL ? .right : .left
        (text as NSString).draw(in: box, withAttributes: [.font: font, .foregroundColor: NSColor.white,
                                                         .paragraphStyle: paragraph, .kern: spacing])
        return true
    }
    let image = NSImage(size: rect.size, flipped: true) { box in
        NativeColors.gradient.draw(in: box, angle: 0)
        mask.draw(in: box, from: .zero, operation: .destinationIn, fraction: 1)
        return true
    }
    image.draw(in: rect, from: .zero, operation: .sourceOver, fraction: 1, respectFlipped: false, hints: nil)
}

final class GradientField: NSTextField {
    override var isFlipped: Bool { true }
    var centered = false
    override func draw(_ dirtyRect: NSRect) {
        gradientText(stringValue, in: bounds, font: font ?? .systemFont(ofSize: 20, weight: .bold),
                     centered: centered, spacing: centered ? 5.76 : 0)
    }
}

private final class GlassCanvas: NSView {
    weak var owner: GlassBackdropView?
    override var isFlipped: Bool { owner?.isFlipped ?? false }
    override func hitTest(_ point: NSPoint) -> NSView? { nil }
    override func draw(_ dirtyRect: NSRect) { owner?.drawSurface(dirtyRect) }
}

class GlassBackdropView: NSView {
    let backdrop = NSVisualEffectView()
    private let titlebarFill = GlassCanvas()
    private let canvas = GlassCanvas()
    var material: NSVisualEffectView.Material {
        get { backdrop.material }
        set { backdrop.material = newValue }
    }
    var blendingMode: NSVisualEffectView.BlendingMode {
        get { backdrop.blendingMode }
        set { backdrop.blendingMode = newValue }
    }
    override init(frame frameRect: NSRect) {
        super.init(frame: frameRect)
        material = .sidebar
        blendingMode = .behindWindow
        backdrop.state = .followsWindowActiveState
        for view in [backdrop, canvas] {
            view.frame = bounds
            view.autoresizingMask = [.width, .height]
            addSubview(view)
        }
        canvas.owner = self
    }

    required init?(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }

    func extendBackdropThroughTitlebar(in window: NSWindow) {
        guard let frame = window.contentView?.superview else { return }
        backdrop.removeFromSuperview()
        backdrop.frame = frame.bounds
        frame.addSubview(backdrop, positioned: .below, relativeTo: window.contentView)
        titlebarFill.wantsLayer = true
        titlebarFill.layer?.backgroundColor = NSColor.black.cgColor
        titlebarFill.isHidden = true
        frame.addSubview(titlebarFill, positioned: .below, relativeTo: window.contentView)
    }

    func setGlassTopInset(_ inset: CGFloat) {
        guard let frame = backdrop.superview else { return }
        backdrop.appearance = effectiveAppearance
        let region = inset > 0 ? NSRect(x: 0, y: 0, width: bounds.width, height: bounds.height - inset) : frame.bounds
        backdrop.frame = region
        titlebarFill.frame = NSRect(x: 0, y: bounds.height, width: bounds.width,
                                   height: max(0, frame.bounds.height - bounds.height))
        titlebarFill.isHidden = inset == 0
    }

    override var needsDisplay: Bool {
        get { super.needsDisplay }
        set { super.needsDisplay = newValue; canvas.needsDisplay = newValue }
    }

    func drawSurface(_ dirtyRect: NSRect) {
        if !NativeColors.usesGlass {
            NativeColors.current.background.setFill()
            bounds.fill()
        }
    }
}

final class FlippedView: GlassBackdropView {
    weak var owner: AppDelegate?
    override var isFlipped: Bool { true }
    override func drawSurface(_ dirtyRect: NSRect) {
        super.drawSurface(dirtyRect)
        let c = NativeColors.current
        guard let owner else { return }
        switch owner.currentView {
        case "connected":
            NSColor.black.setFill()
            NSRect(x: 0, y: 0, width: bounds.width, height: owner.homeGlassY).fill()
            let top = NativeColors.onBlack
            // if owner.status["authMethod"] as? String != "remote" {
            if owner.status["authMethod"] as? String == "local" {
                owner.drawArtwork("local", in: directionalRect(NSRect(x: 28, y: 8, width: 16, height: 16), width: bounds.width), tint: NativeColors.appStoreWhite(NativeColors.brandPink))
            } else { owner.drawArtwork("github", in: directionalRect(NSRect(x: 28, y: 8, width: 16, height: 16), width: bounds.width), tint: NativeColors.appStoreWhite(NativeColors.brandPink)) }
            // }
            if !owner.hasHeaderNotice, owner.status["authMethod"] as? String != "local", let (used, total) = owner.usageValues {
                rounded(directionalRect(NSRect(x: 28, y: 66, width: 344, height: 5), width: bounds.width), radius: 3, fill: top.track, border: top.track)
                if used > 0 {
                    let track = NSBezierPath(roundedRect: directionalRect(NSRect(x: 28, y: 66, width: max(3, min(1, used / total) * 344), height: 5), width: bounds.width),
                                             xRadius: 3, yRadius: 3)
                    NativeColors.brandPink.setFill()
                    track.fill()
                }
            }
        case "device":
            rounded(directionalRect(NSRect(x: 24, y: 48, width: 352, height: 160), width: bounds.width), radius: 12, fill: c.input, border: c.border)
        default: break
        }
    }
}

final class NativeCheckboxCell: NSButtonCell {
    override func hitTest(for event: NSEvent, in cellFrame: NSRect, of controlView: NSView) -> NSCell.HitResult {
        let point = controlView.convert(event.locationInWindow, from: nil)
        return cellFrame.contains(point) ? [.contentArea, .trackableArea] : []
    }
}

final class AppearanceSelector: NSPopUpButton {
    override var isFlipped: Bool { true }
    override var acceptsFirstResponder: Bool { isEnabled }
    override var canBecomeKeyView: Bool { isEnabled && !isHiddenOrHasHiddenAncestor }
    override func draw(_ dirtyRect: NSRect) {
        let c = NativeColors.current
        rounded(bounds.insetBy(dx: 1, dy: 1), radius: 8, fill: c.button,
                border: window?.firstResponder === self ? c.accent : c.border)
        nativeText(title, in: directionalRect(NSRect(x: 10, y: 6, width: bounds.width - 36, height: 20), width: bounds.width),
                   font: font ?? .systemFont(ofSize: 14), color: isEnabled ? c.text : c.muted)
        let arrow = NSBezierPath()
        for (index, point) in [NSPoint(x: bounds.width - 22, y: 13), NSPoint(x: bounds.width - 18, y: 17),
                               NSPoint(x: bounds.width - 14, y: 13)].enumerated() {
            let point = directionalPoint(point, width: bounds.width)
            if index == 0 { arrow.move(to: point) } else { arrow.line(to: point) }
        }
        arrow.lineWidth = 1.5
        arrow.lineCapStyle = .round
        c.muted.setStroke()
        arrow.stroke()
    }
}

final class ContextSlider: NSSlider {
    override var acceptsFirstResponder: Bool { isEnabled }
    override var canBecomeKeyView: Bool { isEnabled && !isHiddenOrHasHiddenAncestor }
}

class NativeButton: NSButton {
    weak var owner: AppDelegate?
    var key = ""
    var hover = false
    var showsKeyboardFocus = false
    var hoverArea: NSTrackingArea?
    var bridgeStatusText: String {
        L(owner?.pendingBridge == key ? "Working..." : state == .on ? "Active" : "Paused")
    }
    var isPrimaryAction: Bool { key == "addLocal" }
    var bridgeStatusFont: NSFont {
        .systemFont(ofSize: 12)
    }
    override var isFlipped: Bool { true }
    override var acceptsFirstResponder: Bool { isEnabled }
    override var canBecomeKeyView: Bool { isEnabled && !isHiddenOrHasHiddenAncestor }
    override func accessibilityValue() -> Any? {
        if ["codex", "claude", "copilot", "startup", "tray"].contains(key) { return state.rawValue }
        return super.accessibilityValue()
    }
    override func becomeFirstResponder() -> Bool {
        if window?.keyViewSelectionDirection == .selectingNext ||
            window?.keyViewSelectionDirection == .selectingPrevious {
            showsKeyboardFocus = true
        }
        needsDisplay = true
        (superview as? AccountRow)?.redraw()
        return super.becomeFirstResponder()
    }
    override func resignFirstResponder() -> Bool {
        let result = super.resignFirstResponder()
        needsDisplay = true
        (superview as? AccountRow)?.redraw()
        return result
    }
    override func keyDown(with event: NSEvent) {
        if ["accountChoice", "accountRemove", "accountAddGitHub", "accountAddLocal"].contains(key), event.keyCode == 36 {
            performClick(nil)
            return
        }
        if let row = superview as? AccountRow, [125, 126].contains(event.keyCode) {
            row.moveFocus(event.keyCode == 125 ? 1 : -1)
            return
        }
        super.keyDown(with: event)
    }
    override func mouseDown(with event: NSEvent) {
        showsKeyboardFocus = false
        needsDisplay = true
        super.mouseDown(with: event)
    }
    override func updateTrackingAreas() {
        super.updateTrackingAreas()
        if let hoverArea { removeTrackingArea(hoverArea) }
        let area = NSTrackingArea(rect: .zero, options: [.mouseEnteredAndExited, .activeInKeyWindow, .inVisibleRect], owner: self, userInfo: nil)
        addTrackingArea(area)
        hoverArea = area
    }
    override func mouseEntered(with event: NSEvent) { hover = true; needsDisplay = true }
    override func mouseExited(with event: NSEvent) { hover = false; needsDisplay = true }
    func drawNavigation(_ symbol: String, color: NSColor) {
        let c = NativeColors.current
        if hover || isHighlighted {
            rounded(bounds, radius: bounds.height / 2, fill: c.hover, border: .clear)
        }
        let configuration = NSImage.SymbolConfiguration(pointSize: 16, weight: .regular)
        let image = NSImage(systemSymbolName: symbol, accessibilityDescription: nil)?.withSymbolConfiguration(configuration)
        let view = NSImageView(frame: NSRect(x: (bounds.width - 16) / 2, y: (bounds.height - 16) / 2, width: 16, height: 16))
        view.image = image
        view.contentTintColor = color
        view.cell?.draw(withFrame: view.frame, in: self)
    }

    override func draw(_ dirtyRect: NSRect) {
        let c = key == "settings" || (owner?.currentView == "connected" && ["disconnect", "accounts"].contains(key))
            ? NativeColors.onBlack : NativeColors.current
        let fg = isEnabled ? c.text : c.muted
        let font = NSFont.systemFont(ofSize: 12, weight: .medium)
        switch key {
        case "accountChoice", "accountRemove":
            if let row = superview as? AccountRow {
                let focused = window?.firstResponder === self
                if row.highlighted {
                    c.hover.setFill()
                    bounds.fill()
                }
                if key == "accountChoice" {
                    if row.selected {
                        let tick = NSBezierPath()
                        tick.move(to: directionalPoint(NSPoint(x: 9, y: 18), width: bounds.width))
                        tick.line(to: directionalPoint(NSPoint(x: 13, y: 22), width: bounds.width))
                        tick.line(to: directionalPoint(NSPoint(x: 20, y: 13), width: bounds.width))
                        tick.lineWidth = 2
                        c.connected.setStroke()
                        tick.stroke()
                    }
                    let local = row.username.hasPrefix("local:")
                    if local {
                        row.owner?.drawArtwork("local", in: directionalRect(NSRect(x: 30, y: 10, width: 16, height: 16), width: bounds.width), tint: fg)
                    }
                    let textX: CGFloat = local ? 54 : 30
                    nativeText(title, in: directionalRect(NSRect(x: textX, y: 9, width: bounds.width - textX - 4, height: 22), width: bounds.width),
                               font: .systemFont(ofSize: 13), color: fg)
                } else if row.highlighted || focused {
                    let cross = NSBezierPath()
                    cross.move(to: directionalPoint(NSPoint(x: 13, y: 13), width: bounds.width))
                    cross.line(to: directionalPoint(NSPoint(x: 21, y: 21), width: bounds.width))
                    cross.move(to: directionalPoint(NSPoint(x: 21, y: 13), width: bounds.width))
                    cross.line(to: directionalPoint(NSPoint(x: 13, y: 21), width: bounds.width))
                    cross.lineWidth = 1.5
                    c.muted.setStroke()
                    cross.stroke()
                }
            }
        case "accounts":
            if owner?.currentView == "connected" {
                if hover || isHighlighted {
                    rounded(bounds.insetBy(dx: 2, dy: 2), radius: 5,
                            fill: c.hover, border: .clear)
                }
                let connected = L("Connected")
                let font = NSFont.systemFont(ofSize: 12)
                let textRight = bounds.width - 26
                let width = min(textRight - 2, ceil((connected as NSString).size(withAttributes: [.font: font]).width))
                nativeText(connected, in: directionalRect(NSRect(x: textRight - width, y: 6, width: width, height: 22), width: bounds.width),
                           font: font, color: c.connected)
                let chevron = NSBezierPath()
                chevron.move(to: directionalPoint(NSPoint(x: bounds.width - 19, y: 12), width: bounds.width))
                chevron.line(to: directionalPoint(NSPoint(x: bounds.width - 14, y: 17), width: bounds.width))
                chevron.line(to: directionalPoint(NSPoint(x: bounds.width - 9, y: 12), width: bounds.width))
                chevron.lineWidth = 1.5
                chevron.lineCapStyle = .round
                c.muted.setStroke()
                chevron.stroke()
            } else {
                nativeText(title, in: directionalRect(NSRect(x: 0, y: 6, width: bounds.width, height: 22), width: bounds.width),
                           font: .systemFont(ofSize: 12), color: c.accent, alignment: .center)
            }
        case "claude", "codex", "copilot":
            rounded(directionalRect(NSRect(x: 10, y: 6, width: 32, height: 32), width: bounds.width), radius: 8, fill: c.tile, border: c.border)
            let iconSize: CGFloat = key == "claude" ? 20 : 24
            owner?.drawArtwork(key == "copilot" ? "github" : key, in: directionalRect(NSRect(x: 26 - iconSize / 2, y: 22 - iconSize / 2, width: iconSize, height: iconSize), width: bounds.width),
                               tint: key == "claude" ? NativeColors.rgb(0xd97757) : key == "copilot" ? NativeColors.appStoreWhite(c.accent) : c.text)
            nativeText(title, in: directionalRect(NSRect(x: 54, y: 2, width: 180, height: 18), width: bounds.width), font: .systemFont(ofSize: 12), color: c.muted)
            let on = state == .on
            let statusColor = on ? c.connected : c.disconnected
            rounded(directionalRect(NSRect(x: 58, y: 23, width: 8, height: 8), width: bounds.width), radius: 4, fill: statusColor, border: statusColor)
            nativeText(bridgeStatusText, in: directionalRect(NSRect(x: 74, y: 18, width: 180, height: 18), width: bounds.width),
                       font: bridgeStatusFont, color: fg)
            let trackX = bounds.width - 54
            rounded(directionalRect(NSRect(x: trackX, y: 10, width: 44, height: 24), width: bounds.width), radius: 12, fill: on ? c.on : c.track, border: c.border)
            rounded(directionalRect(NSRect(x: trackX + (on ? 22 : 2), y: 13, width: 18, height: 18), width: bounds.width), radius: 9, fill: .white, border: .white)
        case "startup", "tray":
            nativeText(title, in: directionalRect(NSRect(x: 8, y: 7, width: bounds.width - 48, height: 18), width: bounds.width), font: .systemFont(ofSize: 12), color: fg)
            let checkX = bounds.width - 28
            rounded(directionalRect(NSRect(x: checkX, y: 6, width: 20, height: 20), width: bounds.width), radius: 4,
                    fill: state == .on ? c.on : c.button, border: state == .on ? c.on : c.muted)
            if state == .on {
                let tick = NSBezierPath()
                tick.move(to: directionalPoint(NSPoint(x: checkX + 4, y: 16), width: bounds.width))
                tick.line(to: directionalPoint(NSPoint(x: checkX + 8, y: 20), width: bounds.width))
                tick.line(to: directionalPoint(NSPoint(x: checkX + 16, y: 11), width: bounds.width))
                tick.lineWidth = 2
                NSColor.white.setStroke()
                tick.stroke()
            }
        case "browser", "project":
            nativeText(title, in: directionalRect(NSRect(x: 0, y: 4, width: bounds.width, height: bounds.height - 4), width: bounds.width),
                       font: .systemFont(ofSize: key == "project" ? 10 : 12),
                       color: key == "project" ? c.muted : c.accent,
                       alignment: key == "project" ? .right : key == "browser" ? .left : .center)
        case "cancelAccount" where ["add-account", "device"].contains(owner?.currentView ?? ""):
            drawNavigation(L10n.isRTL ? "chevron.right" : "chevron.left", color: fg)
        case "settingsBack":
            drawNavigation(L10n.isRTL ? "chevron.right" : "chevron.left", color: c.muted)
        case "settings":
            drawNavigation("gearshape", color: c.muted)
        default:
            let connection = ["connect", "disconnect"].contains(key)
            let primary = isPrimaryAction
            let statusColor = key == "disconnect" ? c.connected : c.disconnected
            let textColor = connection ? statusColor : fg
            let fill = primary ? c.on : key == "disconnect" ? c.panel :
                connection ? c.button : hover || isHighlighted ? c.hover : c.button
            let circular = key == "settings" || key == "cancelAccount"
            rounded(bounds, radius: circular ? bounds.height / 2 : min(10, bounds.height / 3), fill: fill,
                    border: primary ? (hover || isHighlighted ? c.accent : c.on) :
                        hover || isHighlighted ? (connection ? statusColor : c.accent) : c.border)
            if circular {
                let image = NSImage(systemSymbolName: key == "settings" ? "gearshape" : "xmark", accessibilityDescription: nil)
                image?.isTemplate = true
                let view = NSImageView(frame: directionalRect(NSRect(x: 8, y: 8, width: 16, height: 16), width: bounds.width))
                view.image = image
                view.contentTintColor = c.muted
                view.cell?.draw(withFrame: view.frame, in: self)
            } else if key == "connect" {
                NSImage(systemSymbolName: "plus", accessibilityDescription: nil)?.draw(in: directionalRect(NSRect(x: 23, y: 12, width: 18, height: 18), width: bounds.width))
                nativeText(title, in: directionalRect(NSRect(x: 48, y: 12, width: bounds.width - 60, height: 22), width: bounds.width), font: font, color: textColor)
            } else {
                nativeText(title, in: directionalRect(NSRect(x: 2, y: bounds.height / 2 - 8, width: bounds.width - 4, height: 22), width: bounds.width),
                           font: key == "disconnect" || key == "copy" ? .systemFont(ofSize: 12) : font,
                           color: primary && isEnabled ? .white : textColor, alignment: .center)
            }
        }
        if window?.firstResponder === self && showsKeyboardFocus {
            let rect = ["startup", "tray"].contains(key) ? directionalRect(NSRect(x: bounds.width - 32, y: 2, width: 28, height: 28), width: bounds.width) :
                ["claude", "codex", "copilot"].contains(key) ? directionalRect(NSRect(x: bounds.width - 59, y: 6, width: 52, height: 32), width: bounds.width) :
                bounds.insetBy(dx: 2, dy: 2)
            let radius = key == "settings" || (key == "cancelAccount" && !["add-account", "device"].contains(owner?.currentView ?? "")) ? rect.height / 2 : 6
            let ring = NSBezierPath(roundedRect: rect, xRadius: radius, yRadius: radius)
            ring.lineWidth = 1
            c.accent.setStroke()
            ring.stroke()
        }
    }
}

final class AccountChoiceButton: NativeButton {
    override func accessibilityValue() -> Any? {
        guard let row = superview as? AccountRow else { return super.accessibilityValue() }
        return L(row.selected ? "Selected" : "Not selected")
    }
}

final class AccountListView: NSView {
    var onCancel: (() -> Void)?
    override var acceptsFirstResponder: Bool { true }
    override var isFlipped: Bool { true }
    override func cancelOperation(_ sender: Any?) {
        onCancel?()
    }
}

final class AccountRow: NSView {
    weak var owner: AppDelegate?
    let username: String
    let selected: Bool
    let displayName: String
    let choose = AccountChoiceButton()
    let remove = NativeButton()
    var hovering = false
    var area: NSTrackingArea?
    override var isFlipped: Bool { true }
    var highlighted: Bool {
        hovering || window?.firstResponder === choose || window?.firstResponder === remove
    }
    init(username: String, displayName: String? = nil, selected: Bool, owner: AppDelegate, y: CGFloat) {
        self.username = username
        self.displayName = displayName ?? "@\(username)"
        self.selected = selected
        self.owner = owner
        super.init(frame: NSRect(x: 0, y: y, width: 264, height: 36))
        for (button, key, label, frame, action) in [
            (choose, "accountChoice", self.displayName, NSRect(x: 0, y: 0, width: 228, height: 36), #selector(selectAccount)),
            (remove, "accountRemove", L("Remove account") + ": " + self.displayName, NSRect(x: 228, y: 0, width: 36, height: 36), #selector(removeAccount))
        ] {
            button.key = key
            button.title = label
            button.frame = directionalRect(frame, width: 264)
            button.isBordered = false
            button.focusRingType = .none
            button.target = self
            button.action = action
            button.setAccessibilityLabel(label)
            button.toolTip = label
            addSubview(button)
        }
        choose.setAccessibilityHelp(L(selected ? "Current account" : "Switch to this account"))
        choose.setAccessibilityValue(L(selected ? "Selected" : "Not selected"))
    }
    required init?(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }
    override func updateTrackingAreas() {
        super.updateTrackingAreas()
        if let area { removeTrackingArea(area) }
        let next = NSTrackingArea(rect: .zero, options: [.mouseEnteredAndExited, .activeInKeyWindow, .inVisibleRect],
                                 owner: self, userInfo: nil)
        area = next
        addTrackingArea(next)
    }
    override func mouseEntered(with event: NSEvent) { hovering = true; redraw() }
    override func mouseExited(with event: NSEvent) { hovering = false; redraw() }
    func redraw() { choose.needsDisplay = true; remove.needsDisplay = true }
    func moveFocus(_ offset: Int) {
        guard let rows = superview?.subviews.compactMap({ $0 as? AccountRow }),
              let index = rows.firstIndex(where: { $0 === self }) else { return }
        let next = rows[(index + offset + rows.count) % rows.count]
        next.scrollToVisible(next.bounds)
        window?.makeFirstResponder(next.choose)
        next.choose.showsKeyboardFocus = true
    }
    @objc func selectAccount() { owner?.accountAction("/api/accounts/switch", username) }
    @objc func removeAccount() { owner?.accountAction("/api/accounts/delete", username) }
}

class AppDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate, NSTextFieldDelegate, NSPopoverDelegate, NSToolbarDelegate {
    var window: NSWindow!
    var serverProcess: Process?
    var serverInput: Pipe?
    var statusItem: NSStatusItem?
    var closeToTray = true
    var desktopTimer: Timer?
    var controls: [String: NSView] = [:]
    var authoredTitles: [String: String] = [:]
    var renderedLanguage = ""
    var renderedMenuState = ""
    var layoutIsRTL = false
    var privacyPanel: NSPanel?
    var privacyText: NSTextView?
    var groups: [String: [NSView]] = [:]
    var currentView = "starting"
    var status: [String: Any] = [:]
    var settings: [String: Any] = ["startAtLogin": false, "closeToTray": true]
    var controlToken = UUID().uuidString + UUID().uuidString
    var deviceCode = ""
    var deviceLoading = false
    var deviceRevision = 0
    var verificationURL = ""
    var pollInterval: TimeInterval = 5
    var nextPoll = Date.distantFuture
    var expires = Date.distantFuture
    var busy = false
    var addingAccount = false
    var localModels: [[String: Any]] = []
    var selectedLocalModel = ""
    var localSetupPending = false
    var contextModels: [[String: Any]] = []
    var contextLoading = false
    var contextError = ""
    var contextRevision = 0
    var contextRestartModels: Set<String> = []
    var accountPopover: NSPopover?
    weak var accountMenuAnchor: NSView?
    var pendingBridge: String?
    var refreshing = false
    var terminating = false
    var startupHandled = false
    var bridgeReady = false
    var startupDeadline = Date().addingTimeInterval(10)
    var refreshError = ""
    var notice = ""
    var artwork: [String: NSImage] = [:]
    weak var actionFocus: NSResponder?
    lazy var localSession: URLSession = {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.connectionProxyDictionary = ["HTTPEnable": 0, "HTTPSEnable": 0, "SOCKSEnable": 0]
        configuration.timeoutIntervalForRequest = 3600
        return URLSession(configuration: configuration)
    }()

    func applicationDidFinishLaunching(_ notification: Notification) {
        if let id = Bundle.main.bundleIdentifier,
           let existing = NSRunningApplication.runningApplications(withBundleIdentifier: id)
            .first(where: { $0.processIdentifier != ProcessInfo.processInfo.processIdentifier }) {
            existing.activate(options: [.activateAllWindows, .activateIgnoringOtherApps])
            NSApp.terminate(nil)
            return
        }
        NSApp.setActivationPolicy(.regular)
        setUpMainMenu()
        setUpStatusItem()
        guard createWindow() else { NSApp.terminate(nil); return }
        startServer()
        desktopTimer = Timer.scheduledTimer(withTimeInterval: 2, repeats: true) { [weak self] _ in
            self?.refresh()
            self?.pollDevice()
        }
        refresh()
    }

    func setUpMainMenu() {
        let main = NSMenu()
        let appItem = NSMenuItem()
        main.addItem(appItem)
        let appMenu = NSMenu()
        appMenu.addItem(withTitle: L("About ModelBridge"), action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
        addProductMenuItems(to: appMenu)
        appMenu.addItem(NSMenuItem.separator())
        appMenu.addItem(withTitle: L("Hide ModelBridge"), action: #selector(NSApplication.hide(_:)), keyEquivalent: "h")
        appMenu.addItem(withTitle: L("Show All"), action: #selector(NSApplication.unhideAllApplications(_:)), keyEquivalent: "")
        appMenu.addItem(NSMenuItem.separator())
        appMenu.addItem(withTitle: L("Quit ModelBridge"), action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        appItem.submenu = appMenu
        let edit = NSMenuItem()
        main.addItem(edit)
        let menu = NSMenu(title: L("Edit"))
        menu.addItem(withTitle: L("Undo"), action: Selector(("undo:")), keyEquivalent: "z")
        let redo = menu.addItem(withTitle: L("Redo"), action: Selector(("redo:")), keyEquivalent: "z")
        redo.keyEquivalentModifierMask = [.command, .shift]
        menu.addItem(NSMenuItem.separator())
        menu.addItem(withTitle: L("Cut"), action: #selector(NSText.cut(_:)), keyEquivalent: "x")
        menu.addItem(withTitle: L("Copy"), action: #selector(NSText.copy(_:)), keyEquivalent: "c")
        menu.addItem(withTitle: L("Paste"), action: #selector(NSText.paste(_:)), keyEquivalent: "v")
        menu.addItem(withTitle: L("Select All"), action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
        edit.submenu = menu
        NSApp.mainMenu = main
    }

    func toolbarDefaultItemIdentifiers(_ toolbar: NSToolbar) -> [NSToolbarItem.Identifier] {
        [.flexibleSpace, NSToolbarItem.Identifier("settings")]
    }

    func toolbarAllowedItemIdentifiers(_ toolbar: NSToolbar) -> [NSToolbarItem.Identifier] {
        toolbarDefaultItemIdentifiers(toolbar)
    }

    func toolbar(_ toolbar: NSToolbar, itemForItemIdentifier identifier: NSToolbarItem.Identifier,
                 willBeInsertedIntoToolbar flag: Bool) -> NSToolbarItem? {
        guard identifier.rawValue == "settings" else { return nil }
        let item = NSToolbarItem(itemIdentifier: identifier)
        item.label = L("Settings")
        let holder = NSView(frame: NSRect(x: 0, y: 0, width: 38, height: 32))
        if let settings = controls["settings"] {
            settings.removeFromSuperview()
            settings.frame = NSRect(x: 0, y: 0, width: 32, height: 32)
            holder.addSubview(settings)
        }
        item.view = holder
        item.isBordered = false
        item.autovalidates = false
        return item
    }

    func setUpStatusItem() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        statusItem?.button?.image = menuBarIcon()
        statusItem?.button?.imagePosition = .imageOnly
        statusItem?.button?.toolTip = "ModelBridge"
        statusItem?.button?.setAccessibilityLabel("ModelBridge")
        statusItem?.menu = makeStatusMenu()
    }

    func makeStatusMenu() -> NSMenu {
        let menu = NSMenu()
        let show = menu.addItem(withTitle: L("Show ModelBridge"), action: #selector(showWindow), keyEquivalent: "")
        show.target = self
        menu.addItem(.separator())
        menu.addItem(withTitle: L("Quit ModelBridge"), action: #selector(NSApplication.terminate(_:)), keyEquivalent: "")
        return menu
    }

    func addProductMenuItems(to menu: NSMenu) {
        menu.addItem(.separator())
        let item = menu.addItem(withTitle: L("Language"), action: nil, keyEquivalent: "")
        let languages = NSMenu(title: L("Language"))
        languages.autoenablesItems = false
        for language in [["code": "auto", "name": L("System Default")]] + L10n.languages {
            let choice = languages.addItem(withTitle: language["name"]!, action: #selector(changeLanguage(_:)), keyEquivalent: "")
            choice.target = self
            choice.representedObject = language["code"]
            choice.state = language["code"] == L10n.selection ? .on : .off
            choice.isEnabled = !busy && !terminating
        }
        item.submenu = languages
        let privacy = menu.addItem(withTitle: L("Privacy Statement"), action: #selector(showPrivacyStatement), keyEquivalent: "")
        privacy.target = self
    }

    @objc func changeLanguage(_ sender: NSMenuItem) {
        if let language = sender.representedObject as? String { selectLanguage(language) }
    }

    @objc func changeSettingsLanguage(_ sender: NSPopUpButton) {
        if let language = sender.selectedItem?.representedObject as? String { selectLanguage(language) }
    }

    func selectLanguage(_ language: String) {
        guard !busy, !terminating, language != L10n.selection else { return }
        accountPopover?.close()
        action("/api/language", ["language": language])
    }

    @objc func showPrivacyStatement() {
        preparePrivacyStatement().makeKeyAndOrderFront(nil)
    }

    @discardableResult func preparePrivacyStatement() -> NSPanel {
        if privacyPanel == nil {
            let panel = NSPanel(contentRect: NSRect(x: 0, y: 0, width: 580, height: 480),
                                styleMask: [.titled, .closable, .resizable], backing: .buffered, defer: false)
            panel.isReleasedWhenClosed = false
            panel.minSize = NSSize(width: 420, height: 320)
            panel.isOpaque = false
            panel.backgroundColor = .clear
            panel.titlebarAppearsTransparent = true
            let surface = GlassBackdropView(frame: panel.contentView!.bounds)
            panel.contentView = surface
            surface.extendBackdropThroughTitlebar(in: panel)
            let scroll = NSScrollView(frame: panel.contentView!.bounds)
            scroll.autoresizingMask = [.width, .height]
            scroll.hasVerticalScroller = true
            scroll.drawsBackground = false
            let text = NSTextView(frame: scroll.bounds)
            text.drawsBackground = false
            text.isEditable = false
            text.isSelectable = true
            text.isRichText = false
            text.font = .systemFont(ofSize: 14)
            text.textContainerInset = NSSize(width: 20, height: 16)
            text.autoresizingMask = [.width]
            text.isVerticallyResizable = true
            text.isHorizontallyResizable = false
            text.textContainer?.widthTracksTextView = true
            scroll.documentView = text
            panel.contentView?.addSubview(scroll)
            privacyPanel = panel
            privacyText = text
            panel.center()
        }
        updatePrivacyStatement()
        return privacyPanel!
    }

    func updatePrivacyStatement() {
        privacyPanel?.title = L("Privacy Statement")
        privacyText?.string = L10n.privacy
        privacyText?.alignment = L10n.isRTL ? .right : .left
        privacyText?.baseWritingDirection = L10n.isRTL ? .rightToLeft : .leftToRight
        privacyText?.setAccessibilityLabel(L("Privacy Statement"))
        privacyText?.textColor = NativeColors.current.text
        privacyPanel?.contentView?.needsDisplay = true
    }

    func applyLocalization() {
        let selection = status["language"] as? String ?? "auto"
        L10n.selection = selection
        let language = L10n.current
        guard language != renderedLanguage else { updateLanguageMenus(); return }
        renderedLanguage = language
        for (key, source) in authoredTitles {
            if let button = controls[key] as? NSButton { button.title = L(source) }
            if let label = controls[key] as? NSTextField { label.stringValue = L(source) }
            controls[key]?.setAccessibilityLabel(L(source))
            controls[key]?.toolTip = L(source)
        }
        for (key, source) in [("localModel", "Local model"), ("localProgress", "Local setup progress"),
                              ("contextSlider", "Context window"), ("appearance", "Appearance"),
                              ("language", "Language")] {
            controls[key]?.setAccessibilityLabel(L(source))
        }
        if let selector = controls["appearance"] as? NSPopUpButton {
            for (item, title) in zip(selector.itemArray, ["Auto", "Light", "Dark"]) { item.title = L(title) }
            selector.toolTip = L("Auto follows your system appearance.")
            selector.setAccessibilityHelp(selector.toolTip)
        }
        if let selector = controls["language"] as? NSPopUpButton { selector.item(at: 0)?.title = L("System Default") }
        updatePrivacyStatement()
        updateLanguageMenus()
    }

    func updateLanguageMenus() {
        let state = "\(L10n.current)|\(L10n.selection)|\(busy)|\(terminating)"
        guard state != renderedMenuState else { return }
        renderedMenuState = state
        setUpMainMenu()
        if statusItem != nil { statusItem?.menu = makeStatusMenu() }
    }

    func add(_ view: NSView, key: String, group: String, y: CGFloat, height: CGFloat = 34,
             x: CGFloat = 32, width: CGFloat = 336) {
        view.frame = NSRect(x: x, y: y, width: width, height: height)
        window.contentView?.addSubview(view)
        controls[key] = view
        groups[group, default: []].append(view)
    }

    func label(_ text: String, key: String, group: String, y: CGFloat, height: CGFloat = 34,
               x: CGFloat = 32, width: CGFloat = 336, size: CGFloat = 14) {
        authoredTitles[key] = text
        let field = NSTextField(wrappingLabelWithString: L(text))
        field.font = .systemFont(ofSize: size)
        field.isSelectable = true
        add(field, key: key, group: group, y: y, height: height, x: x, width: width)
    }

    func button(_ title: String, key: String, group: String, y: CGFloat,
                action: Selector, x: CGFloat = 32, width: CGFloat = 336, height: CGFloat = 34) {
        let button = NativeButton(frame: .zero)
        authoredTitles[key] = title
        button.title = L(title)
        button.target = self
        button.action = action
        button.setButtonType(.momentaryPushIn)
        button.setAccessibilityRole(.button)
        button.owner = self
        button.key = key
        button.setAccessibilityLabel(L(title))
        button.toolTip = L(title)
        if ["connect", "disconnect"].contains(key) {
            let actionTitle = key == "disconnect" ? "Disconnect" :
                key == "login" ? "Connect with access token" : "Add account"
            button.toolTip = L(actionTitle)
            button.setAccessibilityHelp(L(actionTitle))
        }
        button.isBordered = false
        button.focusRingType = .none
        button.font = .systemFont(ofSize: 14)
        add(button, key: key, group: group, y: y, height: height, x: x, width: width)
    }

    func checkbox(_ title: String, key: String, group: String, y: CGFloat, height: CGFloat, action: Selector,
                  x: CGFloat = 32, width: CGFloat = 336) {
        let button = NativeButton(frame: .zero)
        button.cell = NativeCheckboxCell(textCell: title)
        authoredTitles[key] = title
        button.title = L(title)
        button.target = self
        button.action = action
        button.setButtonType(.switch)
        button.setAccessibilityRole(.checkBox)
        button.owner = self
        button.key = key
        button.setAccessibilityLabel(L(title))
        button.focusRingType = .none
        button.font = .systemFont(ofSize: 14)
        add(button, key: key, group: group, y: y, height: height, x: x, width: width)
    }

    @discardableResult func createWindow(show: Bool = true) -> Bool {
        artwork["local"] = NSImage(systemSymbolName: "cpu", accessibilityDescription: nil)
        for name in ["kobashi-icon", "github", "claude", "codex"] {
            guard let url = Bundle.main.url(forResource: name, withExtension: "png"),
                  let image = NSImage(contentsOf: url) else {
                showError("The app is missing its \(name) artwork.")
                return false
            }
            artwork[name] = image
        }
        window = NSWindow(contentRect: NSRect(origin: .zero, size: windowSize),
                          styleMask: [.titled, .closable, .miniaturizable],
                          backing: .buffered, defer: false)
        window.title = "ModelBridge"
        window.titleVisibility = .hidden
        window.delegate = self
        window.isReleasedWhenClosed = false
        window.autorecalculatesKeyViewLoop = true
        window.isOpaque = false
        window.backgroundColor = .clear
        window.titlebarAppearsTransparent = true
        let content = FlippedView(frame: NSRect(origin: .zero, size: windowSize))
        content.owner = self
        window.contentView = content
        content.extendBackdropThroughTitlebar(in: window)
        window.center()
        button("Settings", key: "settings", group: "all", y: 0, action: #selector(settingsView), x: 0, width: 32, height: 32)
        let toolbar = NSToolbar(identifier: "main")
        toolbar.delegate = self
        toolbar.displayMode = .iconOnly
        toolbar.showsBaselineSeparator = false
        window.toolbarStyle = .unified
        window.toolbar = toolbar
        label("Starting...", key: "startingText", group: "starting", y: 163, height: 24, x: 40, width: 320)
        if let starting = controls["startingText"] as? NSTextField {
            starting.alignment = .center
            starting.isSelectable = false
            starting.setAccessibilityElement(true)
            starting.setAccessibilityRole(.staticText)
            starting.setAccessibilityLabel("Starting...")
        }
        button("Not Connected", key: "connect", group: "sign-in", y: 244, action: #selector(addAccount), x: 95, width: 210, height: 42)
        label("Add a local model", key: "addAccountTitle", group: "add-account", y: 8, height: 22, x: 48, width: 328, size: 13)
        let modelPicker = AppearanceSelector(frame: .zero, pullsDown: false)
        modelPicker.font = .systemFont(ofSize: 12)
        modelPicker.target = self
        modelPicker.action = #selector(selectLocalModel(_:))
        modelPicker.setAccessibilityLabel("Local model")
        add(modelPicker, key: "localModel", group: "add-account", y: 48, height: 32, x: 24, width: 352)
        label("", key: "localProgressText", group: "add-account", y: 96, height: 32, x: 24, width: 352, size: 12)
        let progress = NSProgressIndicator()
        progress.style = .bar
        progress.isIndeterminate = false
        progress.minValue = 0
        progress.maxValue = 1
        progress.setAccessibilityLabel("Local setup progress")
        add(progress, key: "localProgress", group: "add-account", y: 136, height: 4, x: 24, width: 352)
        button("Add", key: "addLocal", group: "add-account", y: 156, action: #selector(addLocalModel), x: 24, width: 352, height: 32)
        label("Add GitHub account", key: "githubDeviceTitle", group: "device", y: 8, height: 22, x: 48, width: 328, size: 13)
        label("Enter this code at GitHub", key: "deviceLabel", group: "device", y: 64, height: 18, x: 40, width: 320, size: 12)
        (controls["deviceLabel"] as? NSTextField)?.alignment = .center
        button("https://github.com/login/device", key: "browser", group: "device", y: 88, action: #selector(openGitHub), x: 40, width: 320, height: 20)
        let code = GradientField(labelWithString: "")
        code.font = .monospacedSystemFont(ofSize: 32, weight: .bold)
        code.centered = true
        add(code, key: "code", group: "device", y: 112, height: 42, x: 40, width: 232)
        button("Copy", key: "copy", group: "device", y: 120, action: #selector(copyCode), x: 288, width: 72, height: 28)
        label("", key: "account", group: "connected", y: 8, height: 22, x: 52, width: 186, size: 13)
        button("Accounts", key: "accounts", group: "all", y: 87, action: #selector(showAccounts), x: 321, width: 28, height: 28)
        (controls["accounts"] as? NSButton)?.setAccessibilityLabel("Accounts")
        button("Cancel", key: "cancelAccount", group: "all", y: 310, action: #selector(cancelAddAccount), x: 184, width: 32, height: 32)
        (controls["cancelAccount"] as? NSButton)?.keyEquivalent = "\u{1b}"
        label("AI credits this cycle", key: "usageLabel", group: "connected", y: 42, height: 16, x: 28, width: 182, size: 11)
        label("", key: "usageValue", group: "connected", y: 42, height: 16, x: 210, width: 162, size: 11)
        (controls["usageValue"] as? NSTextField)?.alignment = .right
        let contextSlider = ContextSlider(value: 0, minValue: 0, maxValue: 1, target: self, action: #selector(saveLocalContext(_:)))
        contextSlider.isContinuous = false
        contextSlider.allowsTickMarkValuesOnly = true
        contextSlider.setAccessibilityLabel("Context window")
        add(contextSlider, key: "contextSlider", group: "connected", y: localContextSliderFrame.minY, height: localContextSliderFrame.height, x: localContextSliderFrame.minX, width: localContextSliderFrame.width)
        checkbox("Codex", key: "codex", group: "connected", y: 109, height: 44, action: #selector(toggleCodex), x: 18, width: 364)
        checkbox("Claude Code", key: "claude", group: "connected", y: 153, height: 44, action: #selector(toggleClaude), x: 18, width: 364)
        checkbox("GitHub Copilot", key: "copilot", group: "connected", y: 197, height: 44, action: #selector(toggleCopilot), x: 18, width: 364)
        button("Back", key: "settingsBack", group: "settings", y: 0, action: #selector(done), x: 16, width: 32, height: 32)
        (controls["settingsBack"] as? NSButton)?.keyEquivalent = "\u{1b}"
        label("Settings", key: "settingsTitle", group: "settings", y: 8, height: 22, x: 48, width: 328, size: 13)
        checkbox("Start at login", key: "startup", group: "settings", y: 48, height: 32, action: #selector(saveStartup), x: 16, width: 368)
        checkbox("Close to system tray", key: "tray", group: "settings", y: 84, height: 32, action: #selector(saveTray), x: 16, width: 368)
        label("Appearance", key: "appearanceLabel", group: "settings", y: 131, height: 18, x: 24, width: 152, size: 12)
        let appearance = AppearanceSelector(frame: .zero, pullsDown: false)
        appearance.addItems(withTitles: ["Auto", "Light", "Dark"])
        for (item, mode) in zip(appearance.itemArray, ["system", "light", "dark"]) {
            item.representedObject = mode
        }
        appearance.font = .systemFont(ofSize: 12)
        appearance.target = self
        appearance.action = #selector(changeAppearance(_:))
        appearance.setAccessibilityLabel("Appearance")
        appearance.setAccessibilityHelp("Auto follows your system appearance.")
        appearance.toolTip = "Auto follows your system appearance."
        add(appearance, key: "appearance", group: "settings", y: 124, height: 32, x: 200, width: 176)
        label("Language", key: "languageLabel", group: "settings", y: 171, height: 18, x: 24, width: 152, size: 12)
        let language = AppearanceSelector(frame: .zero, pullsDown: false)
        for option in [["code": "auto", "name": L("System Default")]] + L10n.languages {
            language.addItem(withTitle: option["name"]!)
            language.lastItem?.representedObject = option["code"]
        }
        language.font = .systemFont(ofSize: 12)
        language.target = self
        language.action = #selector(changeSettingsLanguage(_:))
        add(language, key: "language", group: "settings", y: 164, height: 32, x: 200, width: 176)
        /*
        button("Connect to Server", key: "clientView", group: "settings", y: 316, action: #selector(clientView), width: 164, height: 32)
        button("Share this ModelBridge", key: "serverView", group: "settings", y: 316, action: #selector(serverView), x: 204, width: 164, height: 32)
        label("Connect to Server", key: "clientTitle", group: "client", y: 70, height: 28, size: 18)
        label("Share this ModelBridge", key: "serverTitle", group: "server", y: 70, height: 28, size: 18)
        label("Server URL", key: "remoteURLLabel", group: "client", y: 120, height: 20, size: 12)
        label("Server access token", key: "remoteTokenLabel", group: "client", y: 190, height: 20, size: 12)
        label("Listen IP:port", key: "listenLabel", group: "server", y: 120, height: 20, size: 12)
        label("Access token (blank keeps or generates one)", key: "serverTokenLabel", group: "server", y: 190, height: 20, size: 12)
        for (key, group, y, secure, name) in [
            ("remoteURL", "client", 144, false, "Server URL"),
            ("remoteToken", "client", 214, true, "Server access token"),
            ("listen", "server", 144, false, "Listen IP and port"),
            ("serverToken", "server", 214, true, "Server access token")
        ] {
            let input: NSTextField = secure ? NSSecureTextField() : NSTextField()
            input.setAccessibilityLabel(name)
            input.font = .systemFont(ofSize: 13)
            input.delegate = self
            add(input, key: key, group: group, y: CGFloat(y), height: 28, width: 336)
        }
        button("Find Servers", key: "discover", group: "client", y: 268, action: #selector(findServers), width: 164, height: 34)
        button("Connect", key: "remoteConnect", group: "client", y: 268, action: #selector(remoteConnect), x: 204, width: 164, height: 34)
        button("Enable Server", key: "serverToggle", group: "server", y: 268, action: #selector(toggleServer), width: 164, height: 34)
        button("Copy token", key: "serverCopy", group: "server", y: 268, action: #selector(copyServerToken), x: 204, width: 164, height: 34)
        button("Back", key: "networkDone", group: "network", y: 372, action: #selector(networkDone), width: 336, height: 38)
        (controls["networkDone"] as? NSButton)?.keyEquivalent = "\u{1b}"
        */
        label("", key: "notice", group: "all", y: 373, height: 42, size: 12)
        render()
        if show {
            window.makeKeyAndOrderFront(nil)
            NSApp.activate(ignoringOtherApps: true)
        }
        window.makeFirstResponder(currentView == "starting" ? nil : controls["connect"])
        return true
    }

    func startServer() {
        startupDeadline = Date().addingTimeInterval(10)
        guard let resources = Bundle.main.resourceURL else { showError("The app bundle has no resources."); return }
        #if arch(arm64)
        let binary = "kobashi-arm64"
        #else
        let binary = "kobashi-x64"
        #endif
        let process = Process()
        process.executableURL = resources.appendingPathComponent("bin/\(binary)")
        process.arguments = ["--no-open"]
        var environment = ProcessInfo.processInfo.environment
        #if APP_STORE
        environment["HOME"] = NSHomeDirectory()
        #endif
        environment["KOBASHI_APP_PATH"] = Bundle.main.bundleURL.path
        environment["KOBASHI_CONTROL_TOKEN"] = controlToken
        process.environment = environment
        let input = Pipe()
        process.standardInput = input
        serverInput = input
        process.terminationHandler = { [weak self] process in
            DispatchQueue.main.async {
                guard let self else { return }
                if self.terminating {
                    if process.terminationStatus != 0 { self.showError("The native bridge exited with status \(process.terminationStatus).") }
                    NSApp.reply(toApplicationShouldTerminate: true)
                } else {
                    self.notice = "The native bridge stopped (status \(process.terminationStatus)). See ~/.kobashi/kobashi.log."
                    self.render()
                    self.showWindow()
                }
            }
        }
        do { try process.run(); serverProcess = process }
        catch { notice = "Failed to start native bridge: \(error.localizedDescription)"; render(); showError(notice) }
    }

    func request(_ path: String, body: [String: Any]? = nil,
                 completion: @escaping ([String: Any]?, String?) -> Void) {
        guard serverProcess?.isRunning == true else { completion(nil, "The native bridge is not running."); return }
        guard let url = URL(string: "http://127.0.0.1:\(UI_PORT)\(path)") else { completion(nil, "Invalid local URL."); return }
        var request = URLRequest(url: url)
        request.setValue("Bearer \(controlToken)", forHTTPHeaderField: "Authorization")
        if body == nil { request.timeoutInterval = 5 }
        if let body {
            request.httpMethod = "POST"
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            do { request.httpBody = try JSONSerialization.data(withJSONObject: body) }
            catch { completion(nil, error.localizedDescription); return }
        }
        localSession.dataTask(with: request) { [weak self] data, response, error in
            var result: [String: Any]?
            var message = error?.localizedDescription
            if message == nil,
               (response as? HTTPURLResponse)?.value(forHTTPHeaderField: "X-Kobashi-Native") != "1" {
                message = "The native bridge is unavailable. Another app may be using its ports."
            }
            if let data, message == nil {
                do {
                    result = try JSONSerialization.jsonObject(with: data) as? [String: Any]
                    if result == nil { message = "The native bridge returned an invalid response." }
                } catch { message = error.localizedDescription }
                if let response = response as? HTTPURLResponse, response.statusCode != 200 {
                    message = result?["error"] as? String ?? "Native bridge error \(response.statusCode)."
                }
            }
            let finalResult = message == nil ? result : nil
            let finalMessage = message
            DispatchQueue.main.async {
                guard let self else { return }
                let code = (error as? URLError)?.code
                let connectionFailure = code == .cannotConnectToHost || code == .networkConnectionLost || code == .timedOut
                if path == "/api/status", body == nil, connectionFailure,
                   !self.bridgeReady, self.serverProcess?.isRunning == true,
                   Date() < self.startupDeadline, !self.terminating {
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
                        self.request(path, completion: completion)
                    }
                    return
                }
                let detail = connectionFailure
                    ? "Could not connect to ModelBridge's local bridge. See ~/.kobashi/kobashi.log."
                    : finalMessage
                completion(finalResult, detail)
            }
        }.resume()
    }

    func refresh() {
        guard !refreshing, !terminating else { return }
        refreshing = true
        request("/api/status") { [weak self] value, error in
            guard let self else { return }
            if let value {
                self.status = value
                self.bridgeReady = value["ready"] as? Bool != false
                if !self.bridgeReady {
                    if self.currentView != "settings" { self.currentView = "starting" }
                } else if self.currentView == "starting" {
                    self.currentView = value["connected"] as? Bool == true ? "connected" : "idle"
                }
                let local = value["local"] as? [String: Any] ?? [:]
                if self.localSetupPending, local["stage"] as? String == "Ready" {
                    self.localSetupPending = false
                    self.addingAccount = false
                    if self.currentView != "settings" { self.currentView = "connected" }
                    self.notice = ""
                }
                if self.bridgeReady, !self.addingAccount, self.currentView != "settings", value["connected"] as? Bool == true { self.currentView = "connected" }
                // !["settings", "client", "server"].contains(self.currentView)
                if self.bridgeReady, self.currentView == "connected", value["connected"] as? Bool != true { self.currentView = "idle" }
                if self.bridgeReady, self.status["authMethod"] as? String == "local" { self.loadLocalContext() }
            }
            let statusError = error
            self.request("/api/settings") { [weak self] value, error in
                guard let self else { return }
                self.refreshing = false
                self.refreshError = statusError ?? error ?? ""
                if let value {
                    self.settings = value
                    self.closeToTray = value["closeToTray"] as? Bool ?? true
                    if self.bridgeReady, !self.startupHandled {
                        self.startupHandled = true
                        if ProcessInfo.processInfo.arguments.contains("--startup"), self.closeToTray {
                            self.window.orderOut(nil)
                            NSApp.setActivationPolicy(.accessory)
                        }
                    }
                }
                self.render()
            }
        }
    }

    func action(_ path: String, _ body: [String: Any] = [:]) {
        guard !busy, !terminating, currentView != "starting" else { return }
        busy = true
        pendingBridge = ["codex", "claude", "copilot"].first { path == "/api/toggle-\($0)" }
        actionFocus = window.firstResponder
        if let field = controls.values.first(where: {
            ($0 as? NSTextField)?.currentEditor() === window.firstResponder
        }) { actionFocus = field }
        notice = pendingBridge == nil && !["/api/language", "/api/settings", "/api/appearance", "/api/local/context", "/api/poll"].contains(path) ? "Working..." : ""
        render()
        request(path, body: body) { [weak self] value, error in
            guard let self else { return }
            self.busy = false
            self.pendingBridge = nil
            self.notice = error ?? ""
            if let value, error == nil {
                switch path {
                case "/api/local/context":
                    if let id = value["model"] as? String,
                       id == body["model"] as? String,
                       let confirmed = value["contextWindow"] as? Int,
                       confirmed == body["contextWindow"] as? Int,
                       value["activeContextWindow"] as? Int == confirmed,
                       value["contextPending"] as? Bool == false,
                       let index = self.contextModels.firstIndex(where: { $0["id"] as? String == id }) {
                        for key in ["contextWindow", "activeContextWindow", "contextPending"] {
                            self.contextModels[index][key] = value[key]
                        }
                        self.contextRestartModels.insert(id)
                    } else { self.notice = "The bridge did not confirm the context setting." }
                case "/api/poll", "/api/accounts/switch":
                    if value["preparing"] as? Bool == true {
                        self.addingAccount = true
                        self.localSetupPending = true
                        self.currentView = "add-account"
                        self.loadLocalModels()
                    } else if value["ok"] as? Bool == true {
                        self.deviceRevision += 1
                        self.deviceLoading = false
                        self.addingAccount = false
                        self.localSetupPending = false
                        self.deviceCode = ""
                        self.currentView = "connected"
                        self.actionFocus = self.controls["accounts"]
                        self.refresh()
                    } else if value["pending"] as? Bool == true {
                        let reason = value["error"] as? String ?? ""
                        if reason == "slow_down" { self.pollInterval += 5 }
                        else if reason != "authorization_pending", !reason.isEmpty {
                            self.deviceCode = ""
                            self.notice = "GitHub authorization: \(reason)"
                        }
                        self.nextPoll = Date().addingTimeInterval(self.pollInterval)
                    } else {
                        self.notice = ["invalid_token": "The GitHub token is invalid.",
                                       "no_copilot": "This account has no GitHub Copilot access.",
                                       "network": "Could not connect to GitHub."][value["reason"] as? String ?? ""] ?? "Sign-in failed."
                    }
                case "/api/local/add":
                    self.localSetupPending = true
                    self.refresh()
                case "/api/local/cancel":
                    self.localSetupPending = false
                    self.addingAccount = false
                    self.currentView = self.status["connected"] as? Bool == true ? "connected" : "idle"
                    self.refresh()
                case "/api/language":
                    self.status["language"] = value["language"]
                case "/api/settings":
                    self.settings = value
                    self.closeToTray = value["closeToTray"] as? Bool ?? self.closeToTray
                /*
                case "/api/server":
                    (self.controls["serverToken"] as? NSTextField)?.stringValue = value["token"] as? String ?? ""
                    self.notice = "Server settings saved."
                    self.refresh()
                case "/api/remote-connect":
                    (self.controls["remoteToken"] as? NSTextField)?.stringValue = ""
                    self.currentView = "connected"
                    self.actionFocus = self.controls["claude"]
                    self.refresh()
                case "/api/discover":
                    let servers = value["servers"] as? [[String: Any]] ?? []
                    if servers.isEmpty {
                        self.notice = "No Servers found. Enter a Server URL manually."
                    } else if let anchor = self.controls["discover"] {
                        self.render()
                        let menu = NSMenu()
                        for server in servers {
                            guard let url = server["url"] as? String else { continue }
                            let item = NSMenuItem(title: "\(server["name"] as? String ?? "Kobashi") - \(url)",
                                                  action: #selector(self.selectServer(_:)), keyEquivalent: "")
                            item.target = self
                            item.representedObject = url
                            menu.addItem(item)
                        }
                        menu.popUp(positioning: nil, at: NSPoint(x: 0, y: anchor.bounds.height), in: anchor)
                    }
                */
                case "/api/toggle-codex", "/api/toggle-claude", "/api/toggle-copilot":
                    self.status["codexEnabled"] = value["codexEnabled"]
                    self.status["claudeEnabled"] = value["claudeEnabled"]
                    self.status["copilotEnabled"] = value["copilotEnabled"]
                case "/api/disconnect":
                    self.currentView = "idle"
                    self.actionFocus = self.controls["connect"]
                    self.status["connected"] = false
                    self.refresh()
                case "/api/accounts/delete":
                    for key in ["connected", "username", "accounts"] { self.status[key] = value[key] }
                    let accounts = value["accounts"] as? [[String: Any]] ?? []
                    self.currentView = value["connected"] as? Bool == true ? "connected" : "idle"
                    self.actionFocus = self.controls[accounts.isEmpty ? "connect" : "accounts"]
                    self.refresh()
                case "/api/appearance": self.status["appearance"] = value["appearance"]
                default: self.refresh()
                }
            }
            self.render()
            if let focus = self.actionFocus as? NSView, !focus.isHidden {
                self.window.makeFirstResponder(focus)
            }
            if let error { self.showError(error); self.refresh() }
        }
    }

    func setLabel(_ key: String, _ value: String) {
        guard let label = controls[key] as? NSTextField else { return }
        let value = ["notice", "usageLabel"].contains(key) ? L(value) : value
        if label.stringValue != value {
            label.stringValue = value
            label.toolTip = value
            NSAccessibility.post(element: label, notification: .valueChanged)
        }
    }

    var usageValues: (CGFloat, CGFloat)? {
        guard let usage = status["usage"] as? [String: Any], usage["unlimited"] as? Bool != true,
              let used = usage["creditsUsed"] as? NSNumber, let total = usage["entitlement"] as? NSNumber,
              CFGetTypeID(used) != CFBooleanGetTypeID(), CFGetTypeID(total) != CFBooleanGetTypeID(),
              used.doubleValue.isFinite, total.doubleValue.isFinite, used.doubleValue >= 0, total.doubleValue > 0 else { return nil }
        return (CGFloat(used.doubleValue), CGFloat(total.doubleValue))
    }

    func drawArtwork(_ key: String, in rect: NSRect, tint: NSColor? = nil) {
        guard let image = artwork[key] else { return }
        if let tint {
            let colored = NSImage(size: rect.size, flipped: true) { box in
                image.draw(in: box, from: .zero, operation: .sourceOver, fraction: 1, respectFlipped: true, hints: nil)
                tint.setFill()
                box.fill(using: .sourceIn)
                return true
            }
            colored.draw(in: rect, from: .zero, operation: .sourceOver, fraction: 1, respectFlipped: true, hints: nil)
        } else {
            image.draw(in: rect, from: .zero, operation: .sourceOver, fraction: 1, respectFlipped: true, hints: nil)
        }
    }

    func controlTextDidBeginEditing(_ notification: Notification) { window.contentView?.needsDisplay = true }
    func controlTextDidEndEditing(_ notification: Notification) { window.contentView?.needsDisplay = true }

    var hasHeaderNotice: Bool {
        currentView == "connected" && (!notice.isEmpty || !refreshError.isEmpty ||
            !(status["error"] as? String ?? "").isEmpty ||
            (status["authMethod"] as? String == "local" && !contextError.isEmpty))
    }

    var homeGlassY: CGFloat {
        status["authMethod"] as? String == "local" ? localContextSliderFrame.maxY + localContextBottomMargin : mainGlassY
    }

    func render() {
        if layoutIsRTL {
            for (key, view) in controls where key != "settings" { view.frame = NSRect(x: 400 - view.frame.maxX, y: view.frame.minY, width: view.frame.width, height: view.frame.height) }
            layoutIsRTL = false
        }
        applyLocalization()
        let appearance = status["appearance"] as? String ?? "system"
        NSApp.appearance = appearance == "dark" ? NSAppearance(named: .darkAqua) : appearance == "light" ? NSAppearance(named: .aqua) : nil
        window.contentView?.appearance = NSApp.effectiveAppearance
        let colors = NativeColors.current
        updatePrivacyStatement()
        if let selector = controls["appearance"] as? NSPopUpButton {
            selector.selectItem(at: ["system", "light", "dark"].firstIndex(of: appearance) ?? 0)
            selector.isEnabled = !busy
        }
        if let selector = controls["language"] as? NSPopUpButton {
            selector.selectItem(at: selector.itemArray.firstIndex { $0.representedObject as? String == L10n.selection } ?? 0)
            selector.isEnabled = !busy && !terminating
        }
        let homeOffset = homeGlassY - mainGlassY
        let connectedSize = NSSize(width: homeWindowSize.width, height: homeWindowSize.height + homeOffset)
        let size = currentView == "connected" ? connectedSize :
            currentView == "settings" ? settingsWindowSize :
            currentView == "device" ? githubDeviceWindowSize :
            currentView == "add-account" ? addAccountWindowSize : windowSize
        if window.contentView?.bounds.size != size {
            let top = window.frame.maxY
            window.setContentSize(size)
            window.setFrameTopLeftPoint(NSPoint(x: window.frame.minX, y: top))
        }
        (window.contentView as? GlassBackdropView)?.setGlassTopInset(currentView == "connected" ? homeGlassY : 0)
        for (key, y) in [("codex", 109), ("claude", 153), ("copilot", 197)] {
            controls[key]?.frame.origin.y = CGFloat(y) + homeOffset
        }
        controls["connect"]?.frame.origin.y = 160
        let noticeFrame: NSRect
        switch currentView {
        case "starting": noticeFrame = NSRect(x: 40, y: 130, width: 320, height: 90)
        case "connected": noticeFrame = NSRect(x: 28, y: 42, width: 344, height: 42)
        case "add-account": noticeFrame = NSRect(x: 24, y: 96, width: 352, height: 32)
        case "settings": noticeFrame = NSRect(x: 48, y: 8, width: 328, height: 32)
        // case "client", "server": noticeFrame = NSRect(x: 32, y: 316, width: 336, height: 48)
        case "device": noticeFrame = NSRect(x: 40, y: 160, width: 320, height: 32)
        default: noticeFrame = NSRect(x: 32, y: 252, width: 336, height: 70)
        }
        controls["notice"]?.frame = noticeFrame
        for (group, views) in groups {
            let visible = (currentView != "starting" || group == "starting") &&
                (group == "all" || group == currentView ||
                (group == "sign-in" && currentView == "idle"))
                // || (group == "network" && ["client", "server"].contains(currentView))
            for view in views {
                view.isHidden = !visible
                if let button = view as? NSButton {
                    button.isEnabled = !busy && (currentView != "starting" || view === controls["project"])
                }
                let accounts = status["accounts"] as? [[String: Any]] ?? []
                controls["accounts"]?.isHidden = currentView != "connected" && !(currentView == "idle" && !addingAccount && !accounts.isEmpty)
                if currentView == "connected" {
                    let textWidth = ceil((L("Connected") as NSString).size(withAttributes: [.font: NSFont.systemFont(ofSize: 12)]).width)
                    let width = textWidth + 34
                    controls["accounts"]?.frame = NSRect(x: 380 - width, y: 2, width: width, height: 28)
                } else {
                    controls["accounts"]?.frame = NSRect(x: 140, y: 112, width: 120, height: 28)
                }
        (controls["accounts"] as? NSButton)?.isEnabled = !terminating && currentView != "starting"
        updateAccountControls()
                controls["cancelAccount"]?.isHidden = (!addingAccount && currentView != "device") || ["starting", "settings"].contains(currentView)
                if let field = view as? NSTextField { field.textColor = colors.muted }
                view.needsDisplay = true
            }
        }
        if let connect = controls["connect"] as? NSButton {
            connect.title = L("Not Connected")
            connect.setAccessibilityLabel(connect.title)
        }
        if currentView == "device" {
            (controls["copy"] as? NSButton)?.isEnabled = !deviceCode.isEmpty
            (controls["browser"] as? NSButton)?.isEnabled = !deviceCode.isEmpty && !verificationURL.isEmpty
        }
        /*
        for key in ["remoteURL", "remoteToken", "listen", "serverToken"] {
            (controls[key] as? NSTextField)?.isEnabled = !busy
            (controls[key] as? NSTextField)?.textColor = colors.text
        }
        */
        /*
        (controls["remoteConnect"] as? NSButton)?.isEnabled = !busy && !fieldValue("remoteURL").isEmpty && !fieldValue("remoteToken").isEmpty
        let server = status["server"] as? [String: Any] ?? [:]
        let serverAction = server["enabled"] as? Bool == true ? "Disable Server" : "Enable Server"
        (controls["serverToggle"] as? NSButton)?.title = serverAction
        (controls["serverToggle"] as? NSButton)?.setAccessibilityLabel(serverAction)
        */
        for (key, field) in [("codex", "codexEnabled"), ("claude", "claudeEnabled"), ("copilot", "copilotEnabled")] {
            if let button = controls[key] as? NativeButton {
                button.state = status[field] as? Bool == true ? .on : .off
                if button.accessibilityHelp() != button.bridgeStatusText {
                    button.setAccessibilityHelp(button.bridgeStatusText)
                    NSAccessibility.post(element: button, notification: .valueChanged)
                }
            }
        }
        for (key, field) in [("startup", "startAtLogin"), ("tray", "closeToTray")] {
            (controls[key] as? NSButton)?.state = settings[field] as? Bool == true ? .on : .off
        }
        // let prefix = status["authMethod"] as? String == "remote" ? "Server: " : "@"
        let prefix = status["authMethod"] as? String == "local" ? "" : "@"
        setLabel("account", "\(prefix)\(status["username"] as? String ?? "")")
        (controls["account"] as? NSTextField)?.textColor = NativeColors.appStoreWhite(NativeColors.brandPink)
        (controls["settingsTitle"] as? NSTextField)?.textColor = colors.text
        (controls["addAccountTitle"] as? NSTextField)?.textColor = colors.text
        (controls["githubDeviceTitle"] as? NSTextField)?.textColor = colors.text
        (controls["appearanceLabel"] as? NSTextField)?.textColor = colors.text
        (controls["languageLabel"] as? NSTextField)?.textColor = colors.text
        let localAccount = status["authMethod"] as? String == "local"
        for key in ["usageLabel", "usageValue"] {
            controls[key]?.frame.origin.y = 42
            (controls[key] as? NSTextField)?.textColor = NativeColors.onBlack.muted
        }
        for key in ["usageLabel", "usageValue"] { controls[key]?.isHidden = currentView != "connected" || (!localAccount && usageValues == nil) }
        if localAccount {
            setLabel("usageLabel", "Context window")
        } else {
            setLabel("usageLabel", "AI credits this cycle")
            for key in ["usageLabel", "usageValue"] {
                controls[key]?.toolTip = nil
                (controls[key] as? NSTextField)?.setAccessibilityHelp(nil)
            }
        }
        if !localAccount, let (used, total) = usageValues {
            let formatter = NumberFormatter()
            formatter.numberStyle = .decimal
            formatter.locale = L10n.locale
            formatter.maximumFractionDigits = 3
            let usedText = formatter.string(from: NSNumber(value: Double(used))) ?? "\(used)"
            let totalText = formatter.string(from: NSNumber(value: Double(total))) ?? "\(total)"
            setLabel("usageValue", "\(usedText) / \(totalText)")
        }
        var message = notice
        if message.isEmpty { message = refreshError }
        if message.isEmpty, currentView == "connected", localAccount { message = contextError }
        if message.isEmpty { message = status["error"] as? String ?? "" }
        if message.isEmpty, currentView == "settings" { message = settings["error"] as? String ?? "" }
        // if message.isEmpty, currentView == "client" { message = "No GitHub sign-in needed. Disconnect the current source before connecting." }
        // if message.isEmpty, currentView == "server" { message = "Requires GitHub. Share only on a trusted LAN; use HTTPS for public access." }
        if message.isEmpty, currentView == "device" { message = deviceLoading ? "Working..." : deviceCode.isEmpty ? "" : "Waiting for authorization..." }
        setLabel("notice", message)
        controls["notice"]?.isHidden = ["connected", "starting"].contains(currentView) && message.isEmpty
        (controls["notice"] as? NSTextField)?.alignment = currentView == "starting" ? .center : .left
        controls["startingText"]?.isHidden = currentView != "starting" || !message.isEmpty
        controls["settings"]?.isHidden = ["starting", "add-account", "device", "settings"].contains(currentView)
        if let back = controls["cancelAccount"] as? NSButton {
            let isAddAccount = ["add-account", "device"].contains(currentView)
            back.frame.origin = isAddAccount ? NSPoint(x: 16, y: 0) : NSPoint(x: 184, y: 310)
            back.title = L(isAddAccount ? "Back" : "Cancel")
            back.setAccessibilityLabel(back.title)
            back.toolTip = back.title
        }
        controls["settingsTitle"]?.isHidden = currentView != "settings" || !message.isEmpty
        if !message.isEmpty, message != "Working...",
           currentView != "device", !terminating, message != "Changes are saved automatically." {
            (controls["notice"] as? NSTextField)?.textColor = colors.danger
        }
        renderLocalSetup()
        renderLocalContext()
        if hasHeaderNotice {
            for key in ["usageLabel", "usageValue", "contextSlider"] { controls[key]?.isHidden = true }
        }
        let defaultKey = ["add-account": "addLocal", "idle": "connect"][currentView]
        window.defaultButtonCell = defaultKey.flatMap { (controls[$0] as? NSButton)?.cell as? NSButtonCell }
        if L10n.isRTL {
            for (key, view) in controls {
                if key != "settings" { view.frame = directionalRect(view.frame, width: 400) }
                if let field = view as? NSTextField, !["account", "usageValue", "code"].contains(key) {
                    field.alignment = key == "startingText" ? .center : .right
                    field.baseWritingDirection = .rightToLeft
                }
            }
            layoutIsRTL = true
        } else {
            for (key, view) in controls {
                if let field = view as? NSTextField, !["account", "usageValue", "code"].contains(key) {
                    field.alignment = key == "startingText" || (key == "notice" && currentView == "starting") ? .center : .left
                    field.baseWritingDirection = .natural
                }
            }
        }
        window.contentView?.needsDisplay = true
        alignWindowChrome()
    }

    func alignWindowChrome() {
        window.contentView?.superview?.layoutSubtreeIfNeeded()
        guard let close = window.standardWindowButton(.closeButton) else { return }
        let closeRect = close.convert(close.bounds, to: nil)
        let shiftX = homeMargin - closeRect.minX
        let centerY = window.frame.height - windowControlTop - close.bounds.height / 2
        for type in [NSWindow.ButtonType.closeButton, .miniaturizeButton, .zoomButton] {
            guard let control = window.standardWindowButton(type), let parent = control.superview else { continue }
            let center = control.convert(NSPoint(x: control.bounds.midX, y: control.bounds.midY), to: nil)
            control.frame.origin.x += shiftX
            control.frame.origin.y += (parent.isFlipped ? -1 : 1) * (centerY - center.y)
        }
        if let settings = controls["settings"], let parent = settings.superview {
            let center = settings.convert(NSPoint(x: settings.bounds.midX, y: settings.bounds.midY), to: nil)
            let glyphCenterY = window.frame.height - settingsGlyphTop - 8
            settings.frame.origin.x += window.frame.width - settingsGlyphRight - 8 - center.x
            settings.frame.origin.y += (parent.isFlipped ? -1 : 1) * (glyphCenterY - center.y)
        }
    }

    func windowDidResize(_ notification: Notification) { alignWindowChrome() }
    func windowDidBecomeKey(_ notification: Notification) { alignWindowChrome() }

    func pollDevice() {
        guard !deviceCode.isEmpty, !busy, Date() >= nextPoll else { return }
        if Date() >= expires { deviceCode = ""; notice = "GitHub authorization expired. Connect again."; render(); return }
        action("/api/poll", ["device_code": deviceCode])
    }

    @objc func connect() { addGitHubAccount() }
    @objc func toggleCodex() { action("/api/toggle-codex") }
    @objc func toggleClaude() { action("/api/toggle-claude") }
    @objc func toggleCopilot() { action("/api/toggle-copilot") }
    @objc func disconnect() { action("/api/disconnect") }
    @objc func showAccounts() {
        guard let anchor = controls["accounts"] else { return }
        showAccountMenu(from: anchor)
    }
    func showAccountMenu(from anchor: NSView) {
        guard !terminating, currentView != "starting" else { return }
        if accountPopover?.isShown == true { accountPopover?.performClose(nil); return }
        let accounts = status["accounts"] as? [[String: Any]] ?? []
        let listHeight = CGFloat(accounts.count) * 36
        let visibleHeight = min(216, listHeight)
        let content = AccountListView(frame: NSRect(x: 0, y: 0, width: 264, height: visibleHeight + 85))
        content.onCancel = { [weak self] in self?.accountPopover?.performClose(nil) }
        let list = AccountListView(frame: NSRect(x: 0, y: 0, width: 264, height: listHeight))
        list.onCancel = content.onCancel
        for (index, account) in accounts.enumerated() {
            list.addSubview(AccountRow(username: account["username"] as? String ?? "",
                                       displayName: account["displayName"] as? String,
                                       selected: account["selected"] as? Bool == true,
                                       owner: self, y: CGFloat(index) * 36))
        }
        let scroll = NSScrollView(frame: NSRect(x: 0, y: 0, width: 264, height: visibleHeight))
        scroll.documentView = list
        scroll.hasVerticalScroller = true
        scroll.autohidesScrollers = true
        scroll.drawsBackground = false
        content.addSubview(scroll)
        let divider = NSBox(frame: NSRect(x: 8, y: visibleHeight + 4, width: 248, height: 1))
        divider.boxType = .separator
        divider.isHidden = accounts.isEmpty
        content.addSubview(divider)
        let addGitHub = NativeButton(frame: NSRect(x: 8, y: visibleHeight + 9, width: 248, height: 32))
        let addLocal = NativeButton(frame: NSRect(x: 8, y: visibleHeight + 45, width: 248, height: 32))
        for (button, key, title, action) in [
            (addGitHub, "accountAddGitHub", "Add GitHub account", #selector(addGitHubAccount)),
            (addLocal, "accountAddLocal", "Add a local model", #selector(addLocalAccount))
        ] {
            button.key = key
            button.title = L(title)
            button.setAccessibilityLabel(button.title)
            button.isBordered = false
            button.target = self
            button.action = action
            content.addSubview(button)
        }
        let controller = NSViewController()
        controller.view = content
        let popover = NSPopover()
        popover.contentViewController = controller
        popover.contentSize = content.frame.size
        popover.behavior = .transient
        popover.animates = false
        popover.delegate = self
        accountPopover = popover
        accountMenuAnchor = anchor
        updateAccountControls()
        popover.show(relativeTo: anchor.bounds, of: anchor, preferredEdge: .maxY)
        let first = (list.subviews.first as? AccountRow)?.choose ?? addGitHub
        first.showsKeyboardFocus = (anchor as? NativeButton)?.showsKeyboardFocus == true
        content.window?.makeFirstResponder(busy ? content : first)
        anchor.setAccessibilityValue(L("Expanded"))
    }
    func updateAccountControls() {
        guard let content = accountPopover?.contentViewController?.view else { return }
        let enabled = !busy && !terminating
        for child in content.subviews {
            if let button = child as? NSButton { button.isEnabled = enabled }
            if let scroll = child as? NSScrollView {
                for case let row as AccountRow in scroll.documentView?.subviews ?? [] {
                    row.choose.isEnabled = enabled
                    row.remove.isEnabled = enabled
                    row.redraw()
                }
            }
        }
    }
    func popoverDidClose(_ notification: Notification) {
        guard let closed = notification.object as? NSPopover, closed === accountPopover else { return }
        accountPopover = nil
        let anchor = accountMenuAnchor
        accountMenuAnchor = nil
        anchor?.setAccessibilityValue(L("Collapsed"))
        DispatchQueue.main.async { [weak self, weak anchor] in
            guard let self, self.accountPopover == nil, !self.busy,
                  let anchor, !anchor.isHiddenOrHasHiddenAncestor else { return }
            self.window.makeFirstResponder(anchor)
        }
    }
    func accountAction(_ path: String, _ username: String) {
        guard !busy, !terminating else { return }
        accountPopover?.close()
        if path == "/api/accounts/switch", username.hasPrefix("local:") {
            selectedLocalModel = String(username.dropFirst(6))
        }
        action(path, ["username": username])
    }
    @objc func addAccount() {
        guard let anchor = controls["connect"] else { return }
        showAccountMenu(from: anchor)
    }
    @objc func addGitHubAccount() { openGitHubSetup() }
    func openGitHubSetup() {
        guard !busy, !terminating, currentView != "starting" else { return }
        if currentView == "device", addingAccount {
            return
        }
        accountPopover?.close()
        addingAccount = true
        currentView = "device"
        notice = ""
        deviceCode = ""
        verificationURL = ""
        setLabel("code", "—")
        deviceRevision += 1
        let revision = deviceRevision
        deviceLoading = true
        render()
        window.makeFirstResponder(controls["cancelAccount"])
        request("/api/start", body: [:]) { [weak self] value, error in
            guard let self, self.deviceRevision == revision, self.addingAccount,
                  self.currentView == "device" else { return }
            self.deviceLoading = false
            if let value, error == nil,
               let code = value["user_code"] as? String, !code.isEmpty,
               let device = value["device_code"] as? String, !device.isEmpty {
                self.deviceCode = device
                self.verificationURL = value["verification_uri"] as? String ?? ""
                (self.controls["browser"] as? NSButton)?.title = self.verificationURL
                self.setLabel("code", code)
                self.pollInterval = max(1, value["interval"] as? Double ?? 5)
                self.nextPoll = Date().addingTimeInterval(self.pollInterval)
                self.expires = Date().addingTimeInterval(value["expires_in"] as? Double ?? 900)
            } else if self.notice.isEmpty {
                self.notice = error ?? "GitHub did not return a device code."
            }
            self.render()
        }
    }
    @objc func addLocalAccount() {
        guard !busy, !terminating else { return }
        accountPopover?.close()
        addingAccount = true
        currentView = "add-account"
        notice = ""
        selectedLocalModel = ""
        render()
        loadLocalModels()
        window.makeFirstResponder(controls["localModel"])
    }
    func loadLocalModels() {
        request("/api/local/models") { [weak self] value, error in
            guard let self else { return }
            if let value {
                self.localModels = value["models"] as? [[String: Any]] ?? []
                if self.selectedLocalModel.isEmpty { self.selectedLocalModel = value["recommended"] as? String ?? "" }
                if let picker = self.controls["localModel"] as? NSPopUpButton {
                    picker.removeAllItems()
                    picker.autoenablesItems = false
                    for model in self.localModels {
                        let title = (model["name"] as? String ?? "") + (model["recommended"] as? Bool == true ? " · " + L("Recommended") : "")
                        picker.addItem(withTitle: title)
                        picker.lastItem?.representedObject = model["id"]
                        picker.lastItem?.isEnabled = model["available"] as? Bool == true
                        let reason = model["reason"] as? String ?? ""
                        picker.lastItem?.toolTip = L(reason.isEmpty ? model["warning"] as? String ?? "" : reason)
                    }
                    picker.selectItem(at: self.localModels.firstIndex { $0["id"] as? String == self.selectedLocalModel } ?? -1)
                }
                if value["supported"] as? Bool == false { self.notice = value["error"] as? String ?? "Local models are unavailable" }
            } else if let error { self.notice = error }
            self.render()
        }
    }
    @objc func selectLocalModel(_ sender: NSPopUpButton) {
        selectedLocalModel = sender.selectedItem?.representedObject as? String ?? ""
        notice = ""
        render()
    }
    @objc func addLocalModel() {
        guard !selectedLocalModel.isEmpty else { return }
        action("/api/local/add", ["model": selectedLocalModel])
    }
    func renderLocalSetup() {
        guard currentView == "add-account" else { return }
        let local = status["local"] as? [String: Any] ?? [:]
        let stage = local["stage"] as? String ?? ""
        let preparing = localSetupPending && !["Error", "Paused", "Ready"].contains(stage)
        (controls["localModel"] as? NSPopUpButton)?.isEnabled = !busy && !preparing
        let selected = localModels.first { $0["id"] as? String == selectedLocalModel }
        if let add = controls["addLocal"] as? NSButton {
            add.isEnabled = !busy && !preparing && selected?["available"] as? Bool == true
            add.title = L(localSetupPending && stage == "Error" ? "Retry" : "Add")
        }
        var text = ""
        if localSetupPending {
            let downloaded = (local["downloaded"] as? NSNumber)?.int64Value ?? 0
            let total = (local["total"] as? NSNumber)?.int64Value ?? 0
            let completedSteps = (local["completedSteps"] as? NSNumber)?.intValue ?? 0
            let totalSteps = (local["totalSteps"] as? NSNumber)?.intValue ?? 0
            let packages = (local["packages"] as? NSNumber)?.intValue ?? 0
            text = stage
            if stage == "Downloading" { text = "Downloading model" }
            if stage == "Starting" { text = "Starting model" }
            if stage == "Connecting" { text = "Connecting model" }
            if stage == "Installing", let detail = local["detail"] as? String { text = detail }
            text = L(text)
            if totalSteps > 0 {
                let currentStep = min(totalSteps, max(1, completedSteps + 1))
                text = "\(currentStep)/\(totalSteps)  \(text)"
            }
            if total > 0 { text += " · " + ByteCountFormatter.string(fromByteCount: downloaded, countStyle: .file) + " / " + ByteCountFormatter.string(fromByteCount: total, countStyle: .file) }
            else if downloaded > 0 { text += " · " + L10n.format("{0} downloaded", ByteCountFormatter.string(fromByteCount: downloaded, countStyle: .file)) }
            else if packages > 0 { text += " · " + L10n.format("{0} downloaded", packages.formatted(.number.locale(L10n.locale))) }
            if let progress = controls["localProgress"] as? NSProgressIndicator {
                progress.isIndeterminate = total <= 0 && totalSteps <= 0 && preparing
                if progress.isIndeterminate {
                    progress.startAnimation(nil)
                } else {
                    progress.stopAnimation(nil)
                    let bytes = total > 0 ? Double(downloaded) / Double(total) : 0
                    let fraction = totalSteps > 0 ? (Double(completedSteps) + max(0, min(1, bytes))) / Double(totalSteps) : 0
                    progress.doubleValue = max(0, min(1, totalSteps > 0 ? fraction : bytes))
                }
                progress.setAccessibilityHelp(text)
            }
            if stage == "Error" { setLabel("notice", local["error"] as? String ?? "Local setup failed"); (controls["notice"] as? NSTextField)?.textColor = NativeColors.current.danger }
        } else if let bytes = selected?["bytes"] as? NSNumber {
            text = ByteCountFormatter.string(fromByteCount: bytes.int64Value, countStyle: .file)
            if let warning = selected?["warning"] as? String, !warning.isEmpty { text += " · " + L(warning) }
        }
        setLabel("localProgressText", text)
        controls["localProgressText"]?.isHidden = !((controls["notice"] as? NSTextField)?.stringValue.isEmpty ?? true)
        controls["localProgress"]?.isHidden = !preparing
    }
    @objc func cancelAddAccount() {
        guard !busy else { return }
        deviceRevision += 1
        deviceLoading = false
        if localSetupPending { action("/api/local/cancel"); return }
        addingAccount = false
        deviceCode = ""
        currentView = status["connected"] as? Bool == true ? "connected" : "idle"
        notice = ""
        render()
        window.makeFirstResponder(controls["accounts"]?.isHidden == false ? controls["accounts"] : controls["connect"])
    }
    @objc func settingsView() {
        if currentView == "settings" { done(); return }
        currentView = "settings"
        notice = ""
        render()
        window.makeFirstResponder(controls["startup"])
    }
    func loadLocalContext() {
        let id = localContextModelID
        guard !id.isEmpty, !contextLoading, !busy else { return }
        contextLoading = true
        let revision = contextRevision
        request("/api/local/models") { [weak self] value, error in
            guard let self else { return }
            self.contextLoading = false
            guard revision == self.contextRevision, id == self.localContextModelID else { return }
            if let value, value["supported"] as? Bool == true {
                self.contextModels = value["models"] as? [[String: Any]] ?? []
                let model = self.contextModels.first { $0["id"] as? String == id }
                let configured = model?["contextWindow"] as? Int ?? 0
                self.contextError = self.contextChoices.contains(configured) ? "" : "No context settings are available for this model."
            } else {
                self.contextError = error ?? value?["error"] as? String ?? "Local context settings are unavailable."
            }
            self.render()
        }
    }
    var localContextModelID: String { status["localModel"] as? String ?? "" }
    var contextChoices: [Int] {
        contextModels.first { $0["id"] as? String == localContextModelID }?["contextChoices"] as? [Int] ?? []
    }
    func contextTokenText(_ value: Int) -> String {
        L10n.format("{0} tokens", value.formatted(.number.locale(L10n.locale)))
    }
    @objc func saveLocalContext(_ sender: NSSlider) {
        guard currentView == "connected", status["authMethod"] as? String == "local",
              !busy, contextChoices.indices.contains(sender.integerValue) else { return }
        contextRevision += 1
        action("/api/local/context", ["model": localContextModelID, "contextWindow": contextChoices[sender.integerValue]])
    }
    func renderLocalContext() {
        guard let slider = controls["contextSlider"] as? NSSlider else { return }
        slider.isHidden = currentView != "connected" || status["authMethod"] as? String != "local"
        guard !slider.isHidden else {
            if window.firstResponder === slider {
                window.makeFirstResponder(currentView == "connected" ? controls["accounts"] : nil)
            }
            return
        }
        let choices = contextChoices
        let model = contextModels.first { $0["id"] as? String == localContextModelID }
        let configured = model?["contextWindow"] as? Int ?? 0
        let index = choices.firstIndex(of: configured)
        slider.isEnabled = !busy && index != nil
        slider.trackFillColor = NativeColors.current.accent
        guard let index else {
            setLabel("usageValue", "")
            return
        }
        slider.maxValue = Double(choices.count - 1)
        slider.numberOfTickMarks = choices.count
        if !busy { slider.integerValue = index }
        let value = choices[slider.integerValue]
        let text = contextTokenText(value)
        setLabel("usageValue", text)
        slider.setAccessibilityValueDescription(text)
        let active = model?["activeContextWindow"] as? Int ?? configured
        let pending = model?["contextPending"] as? Bool == true
        let recommended = model?["recommendedContextWindow"] as? Int ?? 0
        let applying = busy && actionFocus === slider
        setLabel("usageLabel", applying ? "Working..." : contextRestartModels.contains(localContextModelID)
                 ? "Restart Codex to apply" : value == recommended ? "Context · recommended" : "Context window")
        let applies = L("Context capacity applies immediately. Restart Codex to reload its budget.")
        let recommendation = choices.contains(recommended) ? L10n.format("Recommended for your RAM: {0}. This is a conservative starting point, not a memory guarantee.", contextTokenText(recommended)) + "\n" : ""
        let help = L("Use Left and Right Arrow to choose a token capacity.") + "\n" + recommendation + L10n.format("Range: {0} to {1}.", contextTokenText(choices[0]), contextTokenText(choices[choices.count - 1])) + "\n" + L("More tokens use more memory and may be slower. 4-bit KV compression stays enabled.") + "\n" + (pending ? L10n.format("Active: {0}. Waiting for runtime confirmation.", contextTokenText(active)) : applies)
        slider.toolTip = "\(text)\n\(help)"
        slider.setAccessibilityHelp(help)
        for key in ["usageLabel", "usageValue"] {
            controls[key]?.toolTip = help
            (controls[key] as? NSTextField)?.setAccessibilityHelp(help)
        }
    }
    /*
    func fieldValue(_ key: String) -> String {
        (controls[key] as? NSTextField)?.stringValue.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    }
    @objc func clientView() {
        currentView = "client"
        notice = ""
        (controls["remoteURL"] as? NSTextField)?.stringValue = status["remoteURL"] as? String ?? ""
        render()
        window.makeFirstResponder(controls["remoteURL"])
    }
    @objc func serverView() {
        currentView = "server"
        notice = ""
        let server = status["server"] as? [String: Any] ?? [:]
        (controls["listen"] as? NSTextField)?.stringValue = server["listen"] as? String ?? ""
        render()
        window.makeFirstResponder(controls["listen"])
    }
    @objc func networkDone() {
        (controls["serverToken"] as? NSTextField)?.stringValue = ""
        (controls["remoteToken"] as? NSTextField)?.stringValue = ""
        currentView = "settings"
        notice = ""
        render()
        window.makeFirstResponder(controls["clientView"])
    }
    @objc func findServers() { action("/api/discover") }
    @objc func selectServer(_ sender: NSMenuItem) {
        guard let url = sender.representedObject as? String else { return }
        (controls["remoteURL"] as? NSTextField)?.stringValue = url
        notice = "Server selected. Enter its access token."
        actionFocus = controls["remoteToken"]
        render()
        window.makeFirstResponder(controls["remoteToken"])
    }
    @objc func remoteConnect() {
        action("/api/remote-connect", ["url": fieldValue("remoteURL"), "token": fieldValue("remoteToken")])
    }
    @objc func toggleServer() {
        let server = status["server"] as? [String: Any] ?? [:]
        action("/api/server", ["enabled": server["enabled"] as? Bool != true,
                               "listen": fieldValue("listen"), "token": fieldValue("serverToken")])
    }
    @objc func copyServerToken() {
        request("/api/server") { [weak self] value, error in
            guard let self else { return }
            if let error { self.notice = error }
            else if let token = value?["token"] as? String, !token.isEmpty {
                NSPasteboard.general.clearContents()
                if NSPasteboard.general.setString(token, forType: .string) {
                    self.notice = "Token copied. Share it privately."
                } else { self.notice = "Could not copy the Server token." }
            } else { self.notice = "Enable Server to generate an access token." }
            self.render()
        }
    }
    */
    @objc func saveStartup() { action("/api/settings", ["startAtLogin": (controls["startup"] as? NSButton)?.state == .on]) }
    @objc func saveTray() { action("/api/settings", ["closeToTray": (controls["tray"] as? NSButton)?.state == .on]) }
    @objc func done() {
        if addingAccount { cancelAddAccount(); return }
        deviceCode = ""
        currentView = status["ready"] as? Bool == false ? "starting" : status["connected"] as? Bool == true ? "connected" : "idle"
        notice = ""
        render()
        window.makeFirstResponder(controls["settings"])
    }
    @objc func copyCode() {
        NSPasteboard.general.clearContents()
        if !NSPasteboard.general.setString((controls["code"] as? NSTextField)?.stringValue ?? "", forType: .string) { notice = "Could not write clipboard." }
        else { notice = "" }
        render()
    }
    @objc func openGitHub() {
        guard let url = URL(string: verificationURL), url.scheme == "https" else { notice = "Invalid GitHub verification URL."; render(); return }
        if !NSWorkspace.shared.open(url) { notice = "Could not open GitHub."; render() }
    }
    @objc func openProject() {
        if !NSWorkspace.shared.open(URL(string: "https://github.com/david6ca/Kobashi2")!) {
            notice = "Could not open the project page."
            render()
        }
    }
    @objc func changeAppearance(_ sender: NSPopUpButton) {
        guard let mode = sender.selectedItem?.representedObject as? String,
              mode != (status["appearance"] as? String ?? "system") else { return }
        action("/api/appearance", ["appearance": mode])
    }
    @objc func showWindow() {
        NSApp.setActivationPolicy(.regular)
        if window.isMiniaturized { window.deminiaturize(nil) }
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }
    func windowShouldClose(_ sender: NSWindow) -> Bool {
        if !closeToTray { return true }
        sender.orderOut(nil)
        NSApp.setActivationPolicy(.accessory)
        return false
    }
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool { showWindow(); return true }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { !closeToTray }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard serverProcess?.isRunning == true else { return .terminateNow }
        if terminating { return .terminateLater }
        terminating = true
        desktopTimer?.invalidate()
        notice = "Restoring client configurations..."
        render()
        // EOF requests graceful cleanup from our own native child; never force-kill it.
        serverInput?.fileHandleForWriting.closeFile()
        return .terminateLater
    }
    func applicationWillTerminate(_ notification: Notification) {
        desktopTimer?.invalidate()
        serverInput?.fileHandleForWriting.closeFile()
        localSession.invalidateAndCancel()
        if let item = statusItem { NSStatusBar.system.removeStatusItem(item) }
    }
    func showError(_ message: String) {
        let alert = NSAlert()
        alert.messageText = "ModelBridge"
        alert.informativeText = L(message)
        alert.alertStyle = .warning
        alert.runModal()
    }
}

let app = NSApplication.shared
#if APP_STORE
if CommandLine.arguments.contains("--store-self-test") {
    do { try runAppStoreSelfTest(); exit(0) }
    catch { fputs("App Store self-test failed: \(error)\n", stderr); exit(1) }
}
let delegate = AppStoreDelegate()
#else
let delegate = AppDelegate()
#endif
app.delegate = delegate
app.run()

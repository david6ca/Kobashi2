import Cocoa

let UI_PORT = 18922
let windowSize = NSSize(width: 400, height: 390)
let footerY: CGFloat = 356

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

    static var gradient: NSGradient {
        let high = NSWorkspace.shared.accessibilityDisplayShouldIncreaseContrast
        return NSGradient(starting: high ? current.accent : brandPink,
                          ending: high ? current.secondary : brandOrange)!
    }

    static var current: NativeColors {
        let dark = NSApp.effectiveAppearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
        let values: [UInt32] = dark
            ? [0x191014, 0xfff0f4, 0xbda2ac, 0x261a20, 0x49303a, 0x302129, 0x402934, 0x20151a,
               0xe94f76, 0xff9166, 0xf85149, 0x8d7179, 0xc9355d, 0x33222a, 0x2c1520]
            : [0xfff5f3, 0x3d202a, 0x80616c, 0xfffcfb, 0xf0d8dd, 0xffffff, 0xffe6e5, 0xfff9f7,
               0xb52d52, 0xb94722, 0xdc2626, 0x9c7783, 0xb52d52, 0xffffff, 0xffe7ed]
        let c = values.map(rgb)
        let high = NSWorkspace.shared.accessibilityDisplayShouldIncreaseContrast
        return NativeColors(background: c[0], text: c[1], muted: high ? c[1] : c[2], panel: c[3],
                            border: high ? c[1] : c[4], button: c[5], hover: c[6], input: c[7],
                            accent: c[8], secondary: c[9], danger: c[10], track: c[11],
                            on: c[12], tile: c[13], badge: c[14],
                            connected: high ? c[1] : rgb(dark ? 0x3fb950 : 0x15803d),
                            disconnected: high ? c[1] : rgb(dark ? 0x9ca3af : 0x6b7280))
    }
}

func rounded(_ rect: NSRect, radius: CGFloat, fill: NSColor, border: NSColor) {
    let path = NSBezierPath(roundedRect: rect.insetBy(dx: 0.5, dy: 0.5), xRadius: radius, yRadius: radius)
    fill.setFill()
    path.fill()
    border.setStroke()
    path.lineWidth = 1
    path.stroke()
}

func menuBarIcon() -> NSImage {
    let image = NSImage(size: NSSize(width: 22, height: 18), flipped: false) { _ in
        NSColor.black.setFill()
        NSRect(x: 5, y: 7.5, width: 12, height: 3).fill()
        for x: CGFloat in [1.5, 13.5] {
            NSBezierPath(ovalIn: NSRect(x: x, y: 5.5, width: 7, height: 7)).fill()
        }
        return true
    }
    image.isTemplate = true
    return image
}

func nativeText(_ text: String, in rect: NSRect, font: NSFont, color: NSColor,
                alignment: NSTextAlignment = .left) {
    let paragraph = NSMutableParagraphStyle()
    paragraph.alignment = alignment
    paragraph.lineBreakMode = .byTruncatingTail
    (text as NSString).draw(in: rect, withAttributes: [.font: font, .foregroundColor: color, .paragraphStyle: paragraph])
}

func gradientText(_ text: String, in rect: NSRect, font: NSFont, centered: Bool = false, spacing: CGFloat = 0) {
    let mask = NSImage(size: rect.size, flipped: true) { box in
        let paragraph = NSMutableParagraphStyle()
        paragraph.alignment = centered ? .center : .left
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

final class FlippedView: NSView {
    weak var owner: AppDelegate?
    override var isFlipped: Bool { true }
    override func draw(_ dirtyRect: NSRect) {
        let c = NativeColors.current
        c.background.setFill()
        bounds.fill()
        if owner?.hasHeaderNotice != true {
            owner?.drawArtwork("kobashi-icon", in: NSRect(x: 30, y: 24, width: 28, height: 28))
        }
        guard let owner else { return }
        switch owner.currentView {
        case "connected":
            rounded(NSRect(x: 32, y: 72, width: 336, height: 90), radius: 12, fill: c.panel, border: c.border)
            rounded(NSRect(x: 237, y: 87, width: 112, height: 28), radius: 8, fill: c.panel, border: c.border)
            rounded(NSRect(x: 32, y: 178, width: 336, height: 162), radius: 12, fill: c.panel, border: c.border)
            let account = owner.status["username"] as? String ?? ""
            let width = min(175, ("@\(account)" as NSString).size(withAttributes: [.font: NSFont.systemFont(ofSize: 13)]).width + 38)
            rounded(NSRect(x: 50, y: 87, width: width, height: 27), radius: 13, fill: c.badge, border: c.border)
            // if owner.status["authMethod"] as? String != "remote" {
            if owner.status["authMethod"] as? String == "local" {
                NSImage(systemSymbolName: "desktopcomputer", accessibilityDescription: "Local")?.draw(in: NSRect(x: 55, y: 92, width: 16, height: 16))
            } else { owner.drawArtwork("github", in: NSRect(x: 55, y: 92, width: 16, height: 16), tint: c.accent) }
            // }
            if let (used, total) = owner.usageValues {
                rounded(NSRect(x: 50, y: 146, width: 299, height: 5), radius: 3, fill: c.track, border: c.track)
                if used > 0 {
                    let track = NSBezierPath(roundedRect: NSRect(x: 50, y: 146, width: max(3, min(1, used / total) * 299), height: 5),
                                             xRadius: 3, yRadius: 3)
                    NativeColors.gradient.draw(in: track, angle: 0)
                }
            }
        case "device":
            rounded(NSRect(x: 32, y: 126, width: 336, height: 170), radius: 12, fill: c.input, border: c.border)
        case "token":
            let focus = (owner.controls["token"] as? NSTextField)?.currentEditor() != nil
            rounded(NSRect(x: 60, y: 209, width: 280, height: 40), radius: 10, fill: c.input, border: focus ? c.accent : c.border)
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
    override var acceptsFirstResponder: Bool { isEnabled }
    override var canBecomeKeyView: Bool { isEnabled && !isHiddenOrHasHiddenAncestor }
}

final class NativeButton: NSButton {
    weak var owner: AppDelegate?
    var key = ""
    var hover = false
    var showsKeyboardFocus = false
    var hoverArea: NSTrackingArea?
    var bridgeStatusText: String {
        owner?.pendingBridge == key ? "Working..." : state == .on ? "Active" : "Paused"
    }
    var bridgeStatusFont: NSFont {
        .systemFont(ofSize: 13, weight: .medium)
    }
    override var isFlipped: Bool { true }
    override var acceptsFirstResponder: Bool { isEnabled }
    override var canBecomeKeyView: Bool { isEnabled && !isHiddenOrHasHiddenAncestor }
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
        if ["accountChoice", "accountRemove", "accountAdd"].contains(key), event.keyCode == 36 {
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
    override func draw(_ dirtyRect: NSRect) {
        let c = NativeColors.current
        let fg = isEnabled ? c.text : c.muted
        let font = NSFont.systemFont(ofSize: 14, weight: .semibold)
        switch key {
        case "accountChoice", "accountRemove":
            if let row = superview as? AccountRow {
                let focused = window?.firstResponder === self
                (row.highlighted ? c.hover : c.panel).setFill()
                bounds.fill()
                if key == "accountChoice" {
                    if row.selected {
                        let tick = NSBezierPath()
                        tick.move(to: NSPoint(x: 9, y: 18))
                        tick.line(to: NSPoint(x: 13, y: 22))
                        tick.line(to: NSPoint(x: 20, y: 13))
                        tick.lineWidth = 2
                        c.connected.setStroke()
                        tick.stroke()
                    }
                    nativeText(title, in: NSRect(x: 30, y: 9, width: bounds.width - 34, height: 22),
                               font: .systemFont(ofSize: 13), color: fg)
                } else if row.highlighted || focused {
                    let cross = NSBezierPath()
                    cross.move(to: NSPoint(x: 13, y: 13))
                    cross.line(to: NSPoint(x: 21, y: 21))
                    cross.move(to: NSPoint(x: 21, y: 13))
                    cross.line(to: NSPoint(x: 13, y: 21))
                    cross.lineWidth = 1.5
                    c.muted.setStroke()
                    cross.stroke()
                }
            }
        case "disconnect", "accounts":
            if owner?.currentView == "connected" {
                if hover || isHighlighted {
                    rounded(bounds.insetBy(dx: 2, dy: 2), radius: 5,
                            fill: key == "disconnect" ? c.panel : c.hover,
                            border: key == "disconnect" ? c.connected : c.hover)
                }
                if key == "accounts" {
                    c.border.setFill()
                    NSRect(x: 0, y: 5, width: 1, height: 18).fill()
                    let chevron = NSBezierPath()
                    chevron.move(to: NSPoint(x: 9, y: 12))
                    chevron.line(to: NSPoint(x: 14, y: 17))
                    chevron.line(to: NSPoint(x: 19, y: 12))
                    chevron.lineWidth = 1.5
                    c.muted.setStroke()
                    chevron.stroke()
                } else {
                    nativeText(title, in: NSRect(x: 2, y: 6, width: bounds.width - 4, height: 22),
                               font: .systemFont(ofSize: 12), color: c.connected, alignment: .center)
                }
            } else {
                nativeText(title, in: NSRect(x: 0, y: 6, width: bounds.width, height: 22),
                           font: .systemFont(ofSize: 12), color: c.accent, alignment: .center)
            }
        case "claude", "codex", "copilot":
            c.panel.setFill()
            bounds.fill()
            rounded(NSRect(x: 10, y: 9, width: 32, height: 32), radius: 8, fill: c.tile, border: c.border)
            let iconSize: CGFloat = key == "claude" ? 20 : 24
            owner?.drawArtwork(key == "copilot" ? "github" : key, in: NSRect(x: 26 - iconSize / 2, y: 25 - iconSize / 2, width: iconSize, height: iconSize),
                               tint: key == "claude" ? NativeColors.rgb(0xd97757) : key == "copilot" ? c.accent : c.text)
            nativeText(title, in: NSRect(x: 54, y: 5, width: 180, height: 18), font: .systemFont(ofSize: 12), color: c.muted)
            let on = state == .on
            let statusColor = on ? c.connected : c.disconnected
            rounded(NSRect(x: 58, y: 33, width: 8, height: 8), radius: 4, fill: statusColor, border: statusColor)
            nativeText(bridgeStatusText, in: NSRect(x: 74, y: 27, width: 180, height: 20),
                       font: bridgeStatusFont, color: fg)
            let trackX = bounds.width - 55
            rounded(NSRect(x: trackX, y: 13, width: 44, height: 24), radius: 12, fill: on ? c.on : c.track, border: c.border)
            rounded(NSRect(x: trackX + (on ? 22 : 2), y: 16, width: 18, height: 18), radius: 9, fill: .white, border: .white)
        case "startup", "tray":
            c.background.setFill()
            bounds.fill()
            nativeText(title, in: NSRect(x: 8, y: 4, width: 288, height: 24), font: font, color: fg)
            rounded(NSRect(x: 308, y: 6, width: 20, height: 20), radius: 4,
                    fill: state == .on ? c.on : c.button, border: state == .on ? c.on : c.muted)
            if state == .on {
                let tick = NSBezierPath()
                tick.move(to: NSPoint(x: 312, y: 16))
                tick.line(to: NSPoint(x: 316, y: 20))
                tick.line(to: NSPoint(x: 324, y: 11))
                tick.lineWidth = 2
                NSColor.white.setStroke()
                tick.stroke()
            }
        case "tokenView", "browser", "project":
            nativeText(title, in: NSRect(x: 0, y: 4, width: bounds.width, height: bounds.height - 4),
                       font: .systemFont(ofSize: key == "project" ? 10 : 12),
                       color: key == "project" ? c.muted : c.accent,
                       alignment: key == "project" ? .right : .center)
        default:
            let connection = ["connect", "login", "disconnect"].contains(key)
            let primary = key == "done"
            let statusColor = key == "disconnect" ? c.connected : c.disconnected
            let textColor = connection ? statusColor : fg
            let fill = primary ? c.on : key == "disconnect" ? c.panel :
                connection ? c.button : hover || isHighlighted ? c.hover : c.button
            rounded(bounds, radius: min(10, bounds.height / 3), fill: fill,
                    border: primary ? (hover || isHighlighted ? c.accent : c.on) :
                        hover || isHighlighted ? (connection ? statusColor : c.accent) : c.border)
            if key == "settings" {
                let image = NSImage(systemSymbolName: "gearshape", accessibilityDescription: nil)
                image?.isTemplate = true
                let view = NSImageView(frame: NSRect(x: 8, y: 8, width: 16, height: 16))
                view.image = image
                view.contentTintColor = c.muted
                view.cell?.draw(withFrame: view.frame, in: self)
            } else if key == "connect" {
                NSImage(systemSymbolName: "plus", accessibilityDescription: nil)?.draw(in: NSRect(x: 23, y: 12, width: 18, height: 18))
                nativeText(title, in: NSRect(x: 48, y: 12, width: bounds.width - 60, height: 22), font: font, color: textColor)
            } else {
                nativeText(title, in: NSRect(x: 2, y: bounds.height / 2 - 8, width: bounds.width - 4, height: 22),
                           font: key == "disconnect" || key == "copy" ? .systemFont(ofSize: 12) : font,
                           color: primary && isEnabled ? .white : textColor, alignment: .center)
            }
        }
        if window?.firstResponder === self && showsKeyboardFocus {
            let rect = ["startup", "tray"].contains(key) ? NSRect(x: 304, y: 2, width: 28, height: 28) :
                ["claude", "codex", "copilot"].contains(key) ? NSRect(x: bounds.width - 59, y: 9, width: 52, height: 32) :
                bounds.insetBy(dx: 2, dy: 2)
            let ring = NSBezierPath(roundedRect: rect, xRadius: 6, yRadius: 6)
            ring.lineWidth = 1
            c.accent.setStroke()
            ring.stroke()
        }
    }
}

final class AccountListView: NSView {
    var onCancel: (() -> Void)?
    override var acceptsFirstResponder: Bool { true }
    override var isFlipped: Bool { true }
    override func cancelOperation(_ sender: Any?) {
        onCancel?()
    }
    override func draw(_ dirtyRect: NSRect) {
        NativeColors.current.panel.setFill()
        bounds.fill()
    }
}

final class AccountRow: NSView {
    weak var owner: AppDelegate?
    let username: String
    let selected: Bool
    let displayName: String
    let choose = NativeButton()
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
            (remove, "accountRemove", "Remove \(self.displayName)", NSRect(x: 228, y: 0, width: 36, height: 36), #selector(removeAccount))
        ] {
            button.key = key
            button.title = label
            button.frame = frame
            button.isBordered = false
            button.focusRingType = .none
            button.target = self
            button.action = action
            button.setAccessibilityLabel(label)
            button.toolTip = label
            addSubview(button)
        }
        choose.setAccessibilityHelp(selected ? "Current account" : "Switch to this account")
        choose.setAccessibilityValue(selected ? "Selected" : "Not selected")
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

class AppDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate, NSTextFieldDelegate, NSPopoverDelegate {
    var window: NSWindow!
    var serverProcess: Process?
    var serverInput: Pipe?
    var statusItem: NSStatusItem?
    var closeToTray = true
    var desktopTimer: Timer?
    var controls: [String: NSView] = [:]
    var groups: [String: [NSView]] = [:]
    var currentView = "starting"
    var status: [String: Any] = [:]
    var settings: [String: Any] = ["startAtLogin": false, "closeToTray": true]
    var controlToken = UUID().uuidString + UUID().uuidString
    var deviceCode = ""
    var verificationURL = ""
    var pollInterval: TimeInterval = 5
    var nextPoll = Date.distantFuture
    var expires = Date.distantFuture
    var busy = false
    var addingAccount = false
    var localModels: [[String: Any]] = []
    var selectedLocalModel = ""
    var localSetupPending = false
    var accountPopover: NSPopover?
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
        appMenu.addItem(withTitle: "About Kobashi2", action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
        appMenu.addItem(NSMenuItem.separator())
        appMenu.addItem(withTitle: "Hide Kobashi2", action: #selector(NSApplication.hide(_:)), keyEquivalent: "h")
        appMenu.addItem(withTitle: "Show All", action: #selector(NSApplication.unhideAllApplications(_:)), keyEquivalent: "")
        appMenu.addItem(NSMenuItem.separator())
        appMenu.addItem(withTitle: "Quit Kobashi2", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        appItem.submenu = appMenu
        let edit = NSMenuItem()
        main.addItem(edit)
        let menu = NSMenu(title: "Edit")
        menu.addItem(withTitle: "Undo", action: Selector(("undo:")), keyEquivalent: "z")
        let redo = menu.addItem(withTitle: "Redo", action: Selector(("redo:")), keyEquivalent: "z")
        redo.keyEquivalentModifierMask = [.command, .shift]
        menu.addItem(NSMenuItem.separator())
        menu.addItem(withTitle: "Cut", action: #selector(NSText.cut(_:)), keyEquivalent: "x")
        menu.addItem(withTitle: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
        menu.addItem(withTitle: "Paste", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
        menu.addItem(withTitle: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
        edit.submenu = menu
        NSApp.mainMenu = main
    }

    func setUpStatusItem() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        statusItem?.button?.image = menuBarIcon()
        statusItem?.button?.imagePosition = .imageOnly
        statusItem?.button?.toolTip = "Kobashi2"
        statusItem?.button?.setAccessibilityLabel("Kobashi2")
        let menu = NSMenu()
        let show = menu.addItem(withTitle: "Show Kobashi2", action: #selector(showWindow), keyEquivalent: "")
        show.target = self
        menu.addItem(NSMenuItem.separator())
        menu.addItem(withTitle: "Quit Kobashi2", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "")
        statusItem?.menu = menu
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
        let field = NSTextField(wrappingLabelWithString: text)
        field.font = .systemFont(ofSize: size)
        field.isSelectable = true
        add(field, key: key, group: group, y: y, height: height, x: x, width: width)
    }

    func button(_ title: String, key: String, group: String, y: CGFloat,
                action: Selector, x: CGFloat = 32, width: CGFloat = 336, height: CGFloat = 34) {
        let button = NativeButton(frame: .zero)
        button.title = title
        button.target = self
        button.action = action
        button.setButtonType(.momentaryPushIn)
        button.setAccessibilityRole(.button)
        button.owner = self
        button.key = key
        button.setAccessibilityLabel(title)
        button.toolTip = title
        if ["connect", "login", "disconnect"].contains(key) {
            let actionTitle = key == "disconnect" ? "Disconnect" :
                key == "login" ? "Connect with access token" : "Add account"
            button.toolTip = actionTitle
            button.setAccessibilityHelp(actionTitle)
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
        button.title = title
        button.target = self
        button.action = action
        button.setButtonType(.switch)
        button.setAccessibilityRole(.checkBox)
        button.owner = self
        button.key = key
        button.setAccessibilityLabel(title)
        button.focusRingType = .none
        button.font = .systemFont(ofSize: 14)
        add(button, key: key, group: group, y: y, height: height, x: x, width: width)
    }

    @discardableResult func createWindow() -> Bool {
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
        window.title = "Kobashi2"
        window.delegate = self
        window.isReleasedWhenClosed = false
        window.autorecalculatesKeyViewLoop = true
        let content = FlippedView(frame: NSRect(origin: .zero, size: windowSize))
        content.owner = self
        window.contentView = content
        window.center()
        let title = GradientField(labelWithString: "Kobashi2")
        title.font = .systemFont(ofSize: 20, weight: .bold)
        add(title, key: "title", group: "all", y: 25, height: 27, x: 62, width: 136)
        button("Settings", key: "settings", group: "all", y: 24, action: #selector(settingsView), x: 336, width: 32, height: 32)
        button("Not Connected", key: "connect", group: "sign-in", y: 244, action: #selector(addAccount), x: 95, width: 210, height: 42)
        let hint = "Have an access token?"
        let link = "Paste it instead"
        let hintFont = NSFont.systemFont(ofSize: 12)
        let hintWidth = ceil((hint as NSString).size(withAttributes: [.font: hintFont]).width) + 8
        let linkWidth = ceil((link as NSString).size(withAttributes: [.font: hintFont]).width) + 8
        let hintX = (400 - hintWidth - 6 - linkWidth) / 2
        label(hint, key: "tokenTeaser", group: "idle", y: 215, height: 24, x: hintX, width: hintWidth, size: 12)
        button(link, key: "tokenView", group: "idle", y: 211, action: #selector(tokenView),
               x: hintX + hintWidth + 6, width: linkWidth, height: 24)
        button("GitHub", key: "addGitHub", group: "add-account", y: 112, action: #selector(connect), x: 40, width: 320, height: 38)
        label("Local model", key: "localLabel", group: "add-account", y: 171, height: 20, x: 40, width: 320, size: 13)
        let modelPicker = AppearanceSelector(frame: .zero, pullsDown: false)
        modelPicker.target = self
        modelPicker.action = #selector(selectLocalModel(_:))
        modelPicker.setAccessibilityLabel("Local model")
        add(modelPicker, key: "localModel", group: "add-account", y: 199, height: 32, x: 40, width: 320)
        label("", key: "localProgressText", group: "add-account", y: 242, height: 24, x: 40, width: 320, size: 12)
        let progress = NSProgressIndicator()
        progress.style = .bar
        progress.isIndeterminate = false
        progress.minValue = 0
        progress.maxValue = 1
        add(progress, key: "localProgress", group: "add-account", y: 274, height: 8, x: 40, width: 320)
        button("Add", key: "addLocal", group: "add-account", y: 298, action: #selector(addLocalModel), x: 240, width: 120, height: 34)
        label("Access token", key: "tokenLabel", group: "token", y: 197, height: 12, x: 60, width: 280, size: 11)
        let token = NSSecureTextField()
        token.placeholderString = "Paste your access token"
        token.setAccessibilityLabel("Access token")
        token.isBordered = false
        token.drawsBackground = false
        token.focusRingType = .none
        token.font = .monospacedSystemFont(ofSize: 13, weight: .regular)
        token.delegate = self
        add(token, key: "token", group: "token", y: 218, height: 22, x: 73, width: 254)
        button("Not Connected", key: "login", group: "token", y: 258, action: #selector(login), x: 60, width: 280, height: 40)
        label("Enter this code at GitHub", key: "deviceLabel", group: "device", y: 147, height: 20, x: 53, width: 294, size: 12)
        (controls["deviceLabel"] as? NSTextField)?.alignment = .center
        button("https://github.com/login/device", key: "browser", group: "device", y: 174, action: #selector(openGitHub), x: 70, width: 260, height: 24)
        let code = GradientField(labelWithString: "")
        code.font = .monospacedSystemFont(ofSize: 32, weight: .bold)
        code.centered = true
        add(code, key: "code", group: "device", y: 206, height: 42, x: 53, width: 294)
        button("Copy", key: "copy", group: "device", y: 253, action: #selector(copyCode), x: 166, width: 68, height: 24)
        label("", key: "account", group: "connected", y: 92, height: 22, x: 77, width: 140, size: 13)
        button("Connected", key: "disconnect", group: "connected", y: 87, action: #selector(disconnect), x: 237, width: 84, height: 28)
        button("Accounts", key: "accounts", group: "all", y: 87, action: #selector(showAccounts), x: 321, width: 28, height: 28)
        (controls["accounts"] as? NSButton)?.setAccessibilityLabel("Accounts")
        button("Cancel", key: "cancelAccount", group: "all", y: 70, action: #selector(cancelAddAccount), x: 32, width: 76, height: 28)
        (controls["cancelAccount"] as? NSButton)?.keyEquivalent = "\u{1b}"
        label("AI credits this cycle", key: "usageLabel", group: "connected", y: 128, height: 16, x: 50, width: 170, size: 11)
        label("", key: "usageValue", group: "connected", y: 128, height: 16, x: 220, width: 129, size: 11)
        (controls["usageValue"] as? NSTextField)?.alignment = .right
        checkbox("Codex", key: "codex", group: "connected", y: 184, height: 50, action: #selector(toggleCodex), x: 40, width: 320)
        checkbox("Claude", key: "claude", group: "connected", y: 234, height: 50, action: #selector(toggleClaude), x: 40, width: 320)
        checkbox("Copilot", key: "copilot", group: "connected", y: 284, height: 50, action: #selector(toggleCopilot), x: 40, width: 320)
        label("Settings", key: "settingsTitle", group: "settings", y: 70, height: 32, width: 260, size: 18)
        (controls["settingsTitle"] as? NSTextField)?.font = .systemFont(ofSize: 18, weight: .bold)
        checkbox("Start at login", key: "startup", group: "settings", y: 118, height: 30, action: #selector(saveStartup))
        checkbox("Close to system tray", key: "tray", group: "settings", y: 166, height: 30, action: #selector(saveTray))
        label("Appearance", key: "appearanceLabel", group: "settings", y: 218, height: 24, x: 40, width: 132)
        let appearance = AppearanceSelector(frame: .zero, pullsDown: false)
        appearance.addItems(withTitles: ["Auto", "Light", "Dark"])
        for (item, mode) in zip(appearance.itemArray, ["system", "light", "dark"]) {
            item.representedObject = mode
        }
        appearance.font = .systemFont(ofSize: 14)
        appearance.target = self
        appearance.action = #selector(changeAppearance(_:))
        appearance.setAccessibilityLabel("Appearance")
        appearance.setAccessibilityHelp("Auto follows your system appearance.")
        appearance.toolTip = "Auto follows your system appearance."
        add(appearance, key: "appearance", group: "settings", y: 214, height: 30, x: 188, width: 172)
        /*
        button("Connect to Server", key: "clientView", group: "settings", y: 316, action: #selector(clientView), width: 164, height: 32)
        button("Share this Kobashi2", key: "serverView", group: "settings", y: 316, action: #selector(serverView), x: 204, width: 164, height: 32)
        label("Connect to Server", key: "clientTitle", group: "client", y: 70, height: 28, size: 18)
        label("Share this Kobashi2", key: "serverTitle", group: "server", y: 70, height: 28, size: 18)
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
        button("Done", key: "done", group: "settings", y: 302, action: #selector(done), width: 336, height: 38)
        (controls["done"] as? NSButton)?.keyEquivalent = "\u{1b}"
        button("Version", key: "project", group: "all", y: footerY, action: #selector(openProject), x: 252, width: 116, height: 24)
        render()
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        window.makeFirstResponder(controls["connect"])
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
                    ? "Could not connect to Kobashi2's local bridge. See ~/.kobashi/kobashi.log."
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
        notice = pendingBridge == nil && !["/api/settings", "/api/appearance"].contains(path) ? "Working..." : ""
        render()
        request(path, body: body) { [weak self] value, error in
            guard let self else { return }
            self.busy = false
            self.pendingBridge = nil
            self.notice = error ?? ""
            if let value, error == nil {
                switch path {
                case "/api/start":
                    if let code = value["user_code"] as? String, let device = value["device_code"] as? String {
                        self.deviceCode = device
                        self.verificationURL = value["verification_uri"] as? String ?? ""
                        (self.controls["browser"] as? NSButton)?.title = self.verificationURL
                        self.setLabel("code", code)
                        self.currentView = "device"
                        self.pollInterval = max(1, value["interval"] as? Double ?? 5)
                        self.nextPoll = Date().addingTimeInterval(self.pollInterval)
                        self.expires = Date().addingTimeInterval(value["expires_in"] as? Double ?? 900)
                        self.openGitHub()
                        self.actionFocus = self.controls["copy"]
                    } else { self.notice = "GitHub did not return a device code." }
                case "/api/token-login", "/api/poll", "/api/accounts/switch":
                    if value["preparing"] as? Bool == true {
                        self.addingAccount = true
                        self.localSetupPending = true
                        self.currentView = "add-account"
                        self.loadLocalModels()
                    } else if value["ok"] as? Bool == true {
                        self.addingAccount = false
                        self.localSetupPending = false
                        self.deviceCode = ""
                        (self.controls["token"] as? NSSecureTextField)?.stringValue = ""
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
                    self.currentView = value["next"] as? String == "token" ? "token" : "idle"
                    self.actionFocus = self.controls["connect"]
                    if self.currentView == "token" { self.actionFocus = self.controls["token"] }
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

    func controlTextDidChange(_ notification: Notification) {
        guard !busy, ["token" /*, "remoteURL", "remoteToken" */].contains(where: {
            notification.object as? NSTextField === controls[$0]
        }) else { return }
        notice = ""
        render()
    }

    func controlTextDidBeginEditing(_ notification: Notification) { window.contentView?.needsDisplay = true }
    func controlTextDidEndEditing(_ notification: Notification) { window.contentView?.needsDisplay = true }

    var hasHeaderNotice: Bool {
        currentView == "connected" && (!notice.isEmpty || !refreshError.isEmpty || !(status["error"] as? String ?? "").isEmpty)
    }

    func render() {
        let appearance = status["appearance"] as? String ?? "system"
        NSApp.appearance = appearance == "dark" ? NSAppearance(named: .darkAqua) : appearance == "light" ? NSAppearance(named: .aqua) : nil
        let colors = NativeColors.current
        if let selector = controls["appearance"] as? NSPopUpButton {
            selector.selectItem(at: ["system", "light", "dark"].firstIndex(of: appearance) ?? 0)
            selector.isEnabled = !busy
        }
        let size = windowSize
        if window.contentView?.bounds.size != size {
            let top = window.frame.maxY
            window.setContentSize(size)
            window.setFrameTopLeftPoint(NSPoint(x: window.frame.minX, y: top))
        }
        controls["connect"]?.frame.origin.y = currentView == "token" ? 154 : 160
        let noticeFrame: NSRect
        switch currentView {
        case "connected": noticeFrame = NSRect(x: 32, y: 24, width: 248, height: 32)
        case "add-account": noticeFrame = NSRect(x: 40, y: 298, width: 190, height: 50)
        case "settings": noticeFrame = NSRect(x: 32, y: 70, width: 336, height: 32)
        // case "client", "server": noticeFrame = NSRect(x: 32, y: 316, width: 336, height: 48)
        case "token": noticeFrame = NSRect(x: 32, y: 308, width: 336, height: 40)
        case "device": noticeFrame = NSRect(x: 32, y: 308, width: 336, height: 40)
        default: noticeFrame = NSRect(x: 32, y: 252, width: 336, height: 70)
        }
        controls["notice"]?.frame = noticeFrame
        for (group, views) in groups {
            let visible = group == "all" || group == currentView ||
                (group == "sign-in" && ["idle", "token"].contains(currentView))
                // || (group == "network" && ["client", "server"].contains(currentView))
            for view in views {
                view.isHidden = !visible
                if let button = view as? NSButton, view !== controls["done"] {
                    button.isEnabled = !busy && (currentView != "starting" || view === controls["project"])
                }
                let accounts = status["accounts"] as? [[String: Any]] ?? []
                controls["accounts"]?.isHidden = currentView != "connected" && !(["idle", "token"].contains(currentView) && !addingAccount && !accounts.isEmpty)
                controls["accounts"]?.frame = currentView == "connected"
                    ? NSRect(x: 321, y: 87, width: 28, height: 28)
                    : NSRect(x: 140, y: 112, width: 120, height: 28)
        (controls["accounts"] as? NSButton)?.isEnabled = !terminating && currentView != "starting"
        updateAccountControls()
                controls["cancelAccount"]?.isHidden = !addingAccount || currentView == "settings"
                if let field = view as? NSTextField { field.textColor = colors.muted }
                view.needsDisplay = true
            }
        }
        if let connect = controls["connect"] as? NSButton {
            if currentView == "starting" { connect.isHidden = false }
            connect.title = currentView == "starting"
                ? (!notice.isEmpty || !refreshError.isEmpty ? "Unavailable" : "Starting...") : "Add account"
            connect.setAccessibilityLabel(connect.title)
        }
        let token = controls["token"] as? NSSecureTextField
        token?.isEnabled = !busy
        token?.textColor = colors.text
        /*
        for key in ["remoteURL", "remoteToken", "listen", "serverToken"] {
            (controls[key] as? NSTextField)?.isEnabled = !busy
            (controls[key] as? NSTextField)?.textColor = colors.text
        }
        */
        (controls["login"] as? NSButton)?.isEnabled = !busy && !(token?.stringValue.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ?? true)
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
        (controls["account"] as? NSTextField)?.textColor = colors.accent
        (controls["settingsTitle"] as? NSTextField)?.textColor = colors.text
        (controls["appearanceLabel"] as? NSTextField)?.textColor = colors.text
        let localAccount = status["authMethod"] as? String == "local"
        for key in ["usageLabel", "usageValue"] { controls[key]?.isHidden = currentView != "connected" || (!localAccount && usageValues == nil) }
        if localAccount {
            setLabel("usageLabel", "Local · oMLX")
            setLabel("usageValue", "TurboQuant 4-bit")
            controls["usageValue"]?.toolTip = "TurboQuant 4-bit on supported layers; final attention layer stays at native precision."
        } else { setLabel("usageLabel", "AI credits this cycle") }
        if let (used, total) = usageValues {
            let formatter = NumberFormatter()
            formatter.numberStyle = .decimal
            formatter.locale = Locale(identifier: "en_US")
            formatter.maximumFractionDigits = 3
            let usedText = formatter.string(from: NSNumber(value: Double(used))) ?? "\(used)"
            let totalText = formatter.string(from: NSNumber(value: Double(total))) ?? "\(total)"
            setLabel("usageValue", "\(usedText) / \(totalText)")
        }
        var message = notice
        if message.isEmpty { message = refreshError }
        if message.isEmpty { message = status["error"] as? String ?? "" }
        if message.isEmpty, currentView == "settings" { message = settings["error"] as? String ?? "" }
        // if message.isEmpty, currentView == "client" { message = "No GitHub sign-in needed. Disconnect the current source before connecting." }
        // if message.isEmpty, currentView == "server" { message = "Requires GitHub. Share only on a trusted LAN; use HTTPS for public access." }
        if message.isEmpty, currentView == "device" { message = "Waiting for authorization..." }
        setLabel("notice", message)
        controls["notice"]?.isHidden = currentView == "connected" && message.isEmpty
        controls["title"]?.isHidden = hasHeaderNotice
        controls["settingsTitle"]?.isHidden = currentView != "settings" || !message.isEmpty
        if !message.isEmpty, message != "Working...",
           currentView != "device", !terminating, message != "Changes are saved automatically." {
            (controls["notice"] as? NSTextField)?.textColor = colors.danger
        }
        let version = status["version"] as? String ?? Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? ""
        (controls["project"] as? NSButton)?.title = "v\(version)"
        renderLocalSetup()
        let defaultKey = ["add-account": "addLocal", "idle": "connect", "token": "login", /* "client": "remoteConnect", "server": "serverToggle", */ "settings": "done"][currentView]
        window.defaultButtonCell = defaultKey.flatMap { (controls[$0] as? NSButton)?.cell as? NSButtonCell }
        window.contentView?.needsDisplay = true
    }

    func pollDevice() {
        guard !deviceCode.isEmpty, !busy, Date() >= nextPoll else { return }
        if Date() >= expires { deviceCode = ""; notice = "GitHub authorization expired. Connect again."; render(); return }
        action("/api/poll", ["device_code": deviceCode])
    }

    @objc func connect() { addingAccount = true; action("/api/start") }
    @objc func tokenView() { currentView = "token"; notice = ""; render(); window.makeFirstResponder(controls["token"]) }
    @objc func login() {
        let token = (controls["token"] as? NSSecureTextField)?.stringValue.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if !token.isEmpty { action("/api/token-login", ["token": token]) }
    }
    @objc func toggleCodex() { action("/api/toggle-codex") }
    @objc func toggleClaude() { action("/api/toggle-claude") }
    @objc func toggleCopilot() { action("/api/toggle-copilot") }
    @objc func disconnect() { action("/api/disconnect") }
    @objc func showAccounts() {
        guard !terminating, currentView != "starting", let anchor = controls["accounts"] else { return }
        if accountPopover?.isShown == true { accountPopover?.performClose(nil); return }
        let accounts = status["accounts"] as? [[String: Any]] ?? []
        let listHeight = CGFloat(accounts.count) * 36
        let visibleHeight = min(216, listHeight)
        let content = AccountListView(frame: NSRect(x: 0, y: 0, width: 264, height: visibleHeight + 49))
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
        content.addSubview(scroll)
        let divider = NSBox(frame: NSRect(x: 8, y: visibleHeight + 4, width: 248, height: 1))
        divider.boxType = .separator
        content.addSubview(divider)
        let add = NativeButton(frame: NSRect(x: 8, y: visibleHeight + 9, width: 248, height: 32))
        add.key = "accountAdd"
        add.title = "Add account"
        add.setAccessibilityLabel("Add account")
        add.isBordered = false
        add.target = self
        add.action = #selector(addAccount)
        content.addSubview(add)
        let controller = NSViewController()
        controller.view = content
        let popover = NSPopover()
        popover.contentViewController = controller
        popover.contentSize = content.frame.size
        popover.behavior = .transient
        popover.animates = false
        popover.delegate = self
        accountPopover = popover
        updateAccountControls()
        popover.show(relativeTo: anchor.bounds, of: anchor, preferredEdge: .maxY)
        let first = (list.subviews.first as? AccountRow)?.choose ?? add
        first.showsKeyboardFocus = (anchor as? NativeButton)?.showsKeyboardFocus == true
        content.window?.makeFirstResponder(busy ? content : first)
        anchor.setAccessibilityValue("Expanded")
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
        controls["accounts"]?.setAccessibilityValue("Collapsed")
        DispatchQueue.main.async { [weak self] in
            guard let self, self.accountPopover == nil, !self.busy,
                  let anchor = self.controls["accounts"], !anchor.isHiddenOrHasHiddenAncestor else { return }
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
        guard !busy, !terminating else { return }
        accountPopover?.close()
        addingAccount = true
        currentView = "add-account"
        notice = ""
        selectedLocalModel = ""
        render()
        loadLocalModels()
        window.makeFirstResponder(controls["addGitHub"])
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
                        let title = (model["name"] as? String ?? "") + (model["recommended"] as? Bool == true ? " · Recommended" : "")
                        picker.addItem(withTitle: title)
                        picker.lastItem?.representedObject = model["id"]
                        picker.lastItem?.isEnabled = model["available"] as? Bool == true
                        picker.lastItem?.toolTip = model["reason"] as? String
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
        (controls["addGitHub"] as? NSButton)?.isEnabled = !busy && !preparing
        let selected = localModels.first { $0["id"] as? String == selectedLocalModel }
        if let add = controls["addLocal"] as? NSButton {
            add.isEnabled = !busy && !preparing && selected?["available"] as? Bool == true
            add.title = localSetupPending && stage == "Error" ? "Retry" : "Add"
        }
        var text = ""
        if localSetupPending {
            let downloaded = (local["downloaded"] as? NSNumber)?.int64Value ?? 0
            let total = (local["total"] as? NSNumber)?.int64Value ?? 0
            text = stage
            if total > 0 { text += " · " + ByteCountFormatter.string(fromByteCount: downloaded, countStyle: .file) + " / " + ByteCountFormatter.string(fromByteCount: total, countStyle: .file) }
            if let progress = controls["localProgress"] as? NSProgressIndicator {
                progress.isIndeterminate = total <= 0 && preparing
                if progress.isIndeterminate { progress.startAnimation(nil) } else { progress.stopAnimation(nil); progress.doubleValue = total > 0 ? Double(downloaded) / Double(total) : 0 }
            }
            if stage == "Error" { setLabel("notice", local["error"] as? String ?? "Local setup failed"); (controls["notice"] as? NSTextField)?.textColor = NativeColors.current.danger }
        } else if let bytes = selected?["bytes"] as? NSNumber {
            text = ByteCountFormatter.string(fromByteCount: bytes.int64Value, countStyle: .file)
        }
        setLabel("localProgressText", text)
        controls["localProgress"]?.isHidden = !preparing
    }
    @objc func cancelAddAccount() {
        guard !busy else { return }
        if localSetupPending { action("/api/local/cancel"); return }
        addingAccount = false
        deviceCode = ""
        (controls["token"] as? NSTextField)?.stringValue = ""
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
        alert.messageText = "Kobashi2"
        alert.informativeText = message
        alert.alertStyle = .warning
        alert.runModal()
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.run()

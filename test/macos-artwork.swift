func check(_ condition: @autoclosure () -> Bool, _ message: String) {
    guard condition() else {
        fputs("FAIL: \(message)\n", stderr)
        exit(1)
    }
    print("PASS: \(message)")
}

final class ArtworkCheckView: NSView {
    let subject: AppDelegate
    let key: String
    let tint: NSColor?
    let flippedCoordinates: Bool
    override var isFlipped: Bool { flippedCoordinates }

    init(subject: AppDelegate, key: String, tint: NSColor?, flipped: Bool) {
        self.subject = subject
        self.key = key
        self.tint = tint
        self.flippedCoordinates = flipped
        super.init(frame: NSRect(x: 0, y: 0, width: 64, height: 64))
    }

    required init?(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }

    override func draw(_ dirtyRect: NSRect) {
        subject.drawArtwork(key, in: bounds, tint: tint)
    }
}

func bitmap(_ view: NSView) -> NSBitmapImageRep {
    guard let result = view.bitmapImageRepForCachingDisplay(in: view.bounds) else {
        check(false, "Artwork supports offscreen capture")
        fatalError()
    }
    view.cacheDisplay(in: view.bounds, to: result)
    return result
}

func alphaDifference(_ actual: NSBitmapImageRep, _ expected: NSBitmapImageRep) -> CGFloat {
    check(actual.pixelsWide == expected.pixelsWide && actual.pixelsHigh == expected.pixelsHigh,
          "Artwork capture dimensions match")
    var difference: CGFloat = 0
    for y in 0..<actual.pixelsHigh {
        for x in 0..<actual.pixelsWide {
            difference += abs(actual.colorAt(x: x, y: y)!.alphaComponent -
                              expected.colorAt(x: x, y: y)!.alphaComponent)
        }
    }
    return difference / CGFloat(actual.pixelsWide * actual.pixelsHigh)
}

let application = NSApplication.shared
application.setActivationPolicy(.prohibited)
let subject = AppDelegate()
for key in ["github", "claude", "codex"] {
    guard let url = Bundle.main.url(forResource: key, withExtension: "png"),
          let image = NSImage(contentsOf: url) else {
        check(false, "The original \(key) artwork loads")
        exit(1)
    }
    subject.artwork[key] = image
    let referenceView = NSImageView(frame: NSRect(x: 0, y: 0, width: 64, height: 64))
    referenceView.image = image
    referenceView.imageScaling = .scaleAxesIndependently
    let reference = bitmap(referenceView)
    for appearance in [NSAppearance.Name.aqua, .darkAqua] {
        application.appearance = NSAppearance(named: appearance)
        for flipped in [false, true] {
            for tint: NSColor? in [nil, NativeColors.current.accent] {
                let view = ArtworkCheckView(subject: subject, key: key, tint: tint, flipped: flipped)
                let actual = bitmap(view)
                check(alphaDifference(actual, reference) < 0.01,
                      "\(key) stays upright in \(appearance.rawValue), flipped=\(flipped), tinted=\(tint != nil)")
            }
        }
    }
}
check(application.windows.isEmpty, "Artwork checks never open a window")

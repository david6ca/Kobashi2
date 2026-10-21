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
let mark = menuBarIcon()
check(mark.isTemplate && mark.size == NSSize(width: 22, height: 18),
      "The menu-bar mark uses a fixed-size system template")
let mask = mark.copy() as! NSImage
mask.isTemplate = false
for scale in [1, 2] {
    let imageView = NSImageView(frame: NSRect(x: 0, y: 0, width: 22 * scale, height: 18 * scale))
    imageView.image = mask
    imageView.imageScaling = .scaleAxesIndependently
    let pixels = bitmap(imageView)
    func alpha(_ x: CGFloat, _ y: CGFloat) -> CGFloat {
        pixels.colorAt(x: Int(x / 22 * CGFloat(pixels.pixelsWide)),
                       y: Int(y / 18 * CGFloat(pixels.pixelsHigh)))!.alphaComponent
    }
    check(alpha(5, 9) > 0.99 && alpha(17, 9) > 0.99 && alpha(11, 9) > 0.99 &&
          alpha(5, 7) > 0.99 && alpha(17, 7) > 0.99,
          "The \(scale)x menu-bar mark has two solid circles joined by a line")
    check(alpha(0, 0) == 0 && alpha(11, 5) == 0 && alpha(11, 12) == 0,
          "The \(scale)x menu-bar mark stays transparent outside the connection shape")
}
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

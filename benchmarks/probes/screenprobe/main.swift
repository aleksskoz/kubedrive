// screenprobe times what a desktop client shows, from outside the client, the
// same way for every client in the comparison (benchmarks/README.md).
// It captures only the client's windows with ScreenCaptureKit, reads each frame
// with Vision text recognition and reports when the text first matches.
//
//   screenprobe launch /Applications/Freelens.app --env HOME=/tmp/h --match 'load-\d{6}'
//   screenprobe click Freelens 120,340 --match 'load-\d{6}' --done '50,?010'
//
// launch starts the app (it must not be running) and times its first window and
// the first matching frame from the moment of launch. click activates a running
// app, clicks at a screen point (points, not pixels) and times from the click;
// --when RE --then X,Y (repeatable, in order) clicks as soon as RE shows, for
// clients whose shortest path to the list takes more clicks.
// A frame's time is its capture time; the recognizer skips frames while busy, so
// each match is reported with the time of the last frame that did not match.
// Needs Screen Recording and Accessibility for the terminal running it.
//
// Build: xcrun swiftc -O main.swift -o screenprobe
import AppKit
import CoreMedia
import ScreenCaptureKit
import Vision

func now() -> Double { CMTimeGetSeconds(CMClockGetTime(CMClockGetHostTimeClock())) }
func fail(_ msg: String) -> Never { FileHandle.standardError.write("screenprobe: \(msg)\n".data(using: .utf8)!); exit(1) }

struct Options {
    var mode = "", target = "", point = CGPoint.zero
    var match: NSRegularExpression?, done: NSRegularExpression?
    var timeout = 120.0
    var env: [String: String] = [:]
    var steps: [(NSRegularExpression, CGPoint?)] = []
    var trace: NSRegularExpression?
}

func point(_ s: String) -> CGPoint {
    let xy = s.split(separator: ",").compactMap { Double($0) }
    guard xy.count == 2 else { fail("bad point \(s)") }
    return CGPoint(x: xy[0], y: xy[1])
}

func click(_ p: CGPoint, before: () -> Void = {}) {
    let src = CGEventSource(stateID: .hidSystemState)
    CGEvent(mouseEventSource: src, mouseType: .mouseMoved, mouseCursorPosition: p, mouseButton: .left)?.post(tap: .cghidEventTap)
    usleep(100_000)
    let down = CGEvent(mouseEventSource: src, mouseType: .leftMouseDown, mouseCursorPosition: p, mouseButton: .left)
    let up = CGEvent(mouseEventSource: src, mouseType: .leftMouseUp, mouseCursorPosition: p, mouseButton: .left)
    before()
    down?.post(tap: .cghidEventTap)
    usleep(30_000)
    up?.post(tap: .cghidEventTap)
}

func parse() -> Options {
    var o = Options()
    var a = Array(CommandLine.arguments.dropFirst())
    guard a.count >= 2 else { fail("usage: screenprobe launch APP | click APPNAME X,Y  --match RE [--done RE] [--timeout S]") }
    o.mode = a.removeFirst(); o.target = a.removeFirst()
    if o.mode == "click" {
        guard !a.isEmpty else { fail("click needs X,Y") }
        o.point = point(a.removeFirst())
    }
    while !a.isEmpty {
        let k = a.removeFirst()
        guard !a.isEmpty else { fail("\(k) needs a value") }
        let v = a.removeFirst()
        switch k {
        case "--match": o.match = try? NSRegularExpression(pattern: v)
        case "--done": o.done = try? NSRegularExpression(pattern: v)
        case "--when":
            guard let re = try? NSRegularExpression(pattern: v) else { fail("bad --when") }
            o.steps.append((re, nil))
        case "--then":
            guard let last = o.steps.last, last.1 == nil else { fail("--then must follow --when") }
            o.steps[o.steps.count - 1].1 = point(v)
        case "--trace": o.trace = try? NSRegularExpression(pattern: v)
        case "--timeout": o.timeout = Double(v) ?? o.timeout
        case "--env":
            let kv = v.split(separator: "=", maxSplits: 1).map(String.init)
            guard kv.count == 2 else { fail("--env takes K=V") }
            o.env[kv[0]] = kv[1]
        default: fail("unknown flag \(k)")
        }
    }
    if o.match == nil { fail("--match is required") }
    if o.steps.contains(where: { $0.1 == nil }) { fail("each --when needs a --then") }
    return o
}

// The client's largest on-screen window, in global points.
func window(pid: pid_t) -> CGRect? {
    let info = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] ?? []
    var best: CGRect?
    for w in info where (w[kCGWindowOwnerPID as String] as? pid_t) == pid && (w[kCGWindowLayer as String] as? Int) == 0 {
        guard let b = w[kCGWindowBounds as String] as? [String: CGFloat] else { continue }
        let r = CGRect(x: b["X"] ?? 0, y: b["Y"] ?? 0, width: b["Width"] ?? 0, height: b["Height"] ?? 0)
        if r.width > 300, r.height > 200, r.width * r.height > (best.map { $0.width * $0.height } ?? 0) { best = r }
    }
    return best
}

final class Recorder: NSObject, SCStreamOutput {
    let match: NSRegularExpression, done: NSRegularExpression?
    let t0: Double
    let lock = NSLock()
    var latest: (CVPixelBuffer, Double)?
    var frames = 0
    init(match: NSRegularExpression, done: NSRegularExpression?, t0: Double) { self.match = match; self.done = done; self.t0 = t0 }

    func stream(_ stream: SCStream, didOutputSampleBuffer sb: CMSampleBuffer, of type: SCStreamOutputType) {
        guard type == .screen, sb.isValid, let px = sb.imageBuffer else { return }
        // Idle frames repeat the last one and carry no new content.
        if let att = (CMSampleBufferGetSampleAttachmentsArray(sb, createIfNecessary: false) as? [[SCStreamFrameInfo: Any]])?.first,
           let raw = att[.status] as? Int, SCFrameStatus(rawValue: raw) != .complete { return }
        lock.lock(); latest = (px, CMTimeGetSeconds(sb.presentationTimeStamp)); frames += 1; lock.unlock()
    }

    func take() -> (CVPixelBuffer, Double)? { lock.lock(); defer { latest = nil; lock.unlock() }; return latest }

    func text(_ px: CVPixelBuffer) -> String {
        let req = VNRecognizeTextRequest()
        req.recognitionLevel = .fast
        req.usesLanguageCorrection = false
        try? VNImageRequestHandler(cvPixelBuffer: px).perform([req])
        return (req.results ?? []).compactMap { $0.topCandidates(1).first?.string }.joined(separator: "\n")
    }

    static func has(_ re: NSRegularExpression, _ s: String) -> Bool {
        re.firstMatch(in: s, range: NSRange(s.startIndex..., in: s)) != nil
    }
}

func ms(_ s: Double) -> String { String(format: "%.0f", s * 1000) }

@MainActor
func run() async {
    let o = parse()
    var pid: pid_t = 0
    var t0 = 0.0
    var windowAt: Double?

    if o.mode == "launch" {
        let url = URL(fileURLWithPath: o.target)
        guard let id = Bundle(url: url)?.bundleIdentifier else { fail("no bundle at \(o.target)") }
        if !NSRunningApplication.runningApplications(withBundleIdentifier: id).isEmpty { fail("\(id) is already running") }
        let cfg = NSWorkspace.OpenConfiguration()
        cfg.activates = true
        if !o.env.isEmpty { cfg.environment = o.env }
        t0 = now()
        do { pid = try await NSWorkspace.shared.openApplication(at: url, configuration: cfg).processIdentifier } catch { fail("launch: \(error)") }
    } else if o.mode == "click" {
        guard let app = NSWorkspace.shared.runningApplications.first(where: { $0.localizedName == o.target }) else { fail("\(o.target) is not running") }
        pid = app.processIdentifier
        app.activate()
        try? await Task.sleep(for: .milliseconds(700))
    } else { fail("mode is launch or click") }

    // Wait for the window: ScreenCaptureKit can only follow an app that has one.
    var win: CGRect?
    while win == nil {
        win = window(pid: pid)
        if win == nil {
            if t0 > 0, now() - t0 > o.timeout { fail("no window after \(o.timeout)s") }
            try? await Task.sleep(for: .milliseconds(5))
        }
    }
    if t0 > 0 { windowAt = now() - t0 }

    guard let content = try? await SCShareableContent.excludingDesktopWindows(true, onScreenWindowsOnly: true),
          let scApp = content.applications.first(where: { $0.processID == pid }),
          let display = content.displays.first(where: { $0.frame.intersects(win!) }) ?? content.displays.first
    else { fail("ScreenCaptureKit does not see pid \(pid)") }
    let filter = SCContentFilter(display: display, including: [scApp], exceptingWindows: [])
    let conf = SCStreamConfiguration()
    let scale = CGFloat(filter.pointPixelScale)
    // Capture the window's area only, at full resolution, so text stays legible.
    func frame(_ w: CGRect) {
        let src = w.offsetBy(dx: -display.frame.minX, dy: -display.frame.minY).intersection(CGRect(origin: .zero, size: display.frame.size))
        conf.sourceRect = src
        conf.width = Int(src.width * scale); conf.height = Int(src.height * scale)
    }
    frame(win!)
    conf.minimumFrameInterval = CMTime(value: 1, timescale: 60)
    conf.queueDepth = 5
    conf.showsCursor = false

    let rec = Recorder(match: o.match!, done: o.done, t0: t0)
    let stream = SCStream(filter: filter, configuration: conf, delegate: nil)
    do {
        try stream.addStreamOutput(rec, type: .screen, sampleHandlerQueue: DispatchQueue(label: "frames"))
        try await stream.startCapture()
    } catch { fail("capture: \(error)") }

    if o.mode == "click" {
        // A frame of the current state first, then the click starts the clock.
        while rec.frames == 0 { try? await Task.sleep(for: .milliseconds(10)) }
        try? await Task.sleep(for: .milliseconds(300))
        click(o.point) { t0 = now() }
        _ = rec.take()
    }

    var firstAt: Double?, firstAfter = 0.0, doneAt: Double?, doneAfter = 0.0
    var lastMiss = t0, lastDoneMiss = t0
    var lastText = ""
    var stepAt: [Double] = []
    var lastTrace = ""
    var checked = now()
    while now() - t0 < o.timeout {
        // Splash screens and resizes: follow the client's largest window.
        if now() - checked > 0.2 {
            checked = now()
            if let w = window(pid: pid), w != win {
                win = w
                frame(w)
                try? await stream.updateConfiguration(conf)
            }
        }
        guard let (px, ts) = rec.take() else { try? await Task.sleep(for: .milliseconds(2)); continue }
        if ts < t0 { continue }
        let s = rec.text(px)
        lastText = s
        // --trace RE prints each new value of RE's first group with its time,
        // e.g. a row counter while the list loads.
        if let tr = o.trace, let m = tr.firstMatch(in: s, range: NSRange(s.startIndex..., in: s)),
           m.numberOfRanges > 1, let r = Range(m.range(at: 1), in: s), String(s[r]) != lastTrace {
            lastTrace = String(s[r])
            print(String(format: "trace %.0f %@", (ts - t0) * 1000, lastTrace))
        }
        if stepAt.count < o.steps.count {
            let (w, p) = o.steps[stepAt.count]
            if Recorder.has(w, s) { stepAt.append(now() - t0); click(p!); _ = rec.take() }
            lastMiss = ts; lastDoneMiss = ts
            continue
        }
        if firstAt == nil {
            if Recorder.has(o.match!, s) { firstAt = ts - t0; firstAfter = lastMiss - t0 } else { lastMiss = ts }
        }
        if let d = o.done, doneAt == nil {
            if Recorder.has(d, s) { doneAt = ts - t0; doneAfter = lastDoneMiss - t0 } else { lastDoneMiss = ts }
        }
        if firstAt != nil, o.done == nil || doneAt != nil { break }
    }
    try? await stream.stopCapture()

    var out = ["pid=\(pid)", "frames=\(rec.frames)"]
    if let w = windowAt { out.append("window_ms=\(ms(w))") }
    for (i, t) in stepAt.enumerated() { out.append("step\(i + 1)_ms=\(ms(t))") }
    if let f = firstAt { out.append("match_ms=\(ms(f)) (after \(ms(firstAfter)))") } else { out.append("match_ms=timeout") }
    if o.done != nil { out.append(doneAt.map { "done_ms=\(ms($0)) (after \(ms(doneAfter)))" } ?? "done_ms=timeout") }
    print(out.joined(separator: " "))
    if firstAt == nil || (o.done != nil && doneAt == nil) {
        FileHandle.standardError.write("last text seen:\n\(lastText.prefix(600))\n".data(using: .utf8)!)
        exit(2)
    }
    exit(0)
}

Task { await run() }
RunLoop.main.run()

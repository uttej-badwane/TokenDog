import AppKit

// `--selftest` runs the same data path the menu bar uses (locate `td`, run
// `td spend --json`, decode it) and prints the result without starting the UI.
// Handy for CI and for users checking their `td` install works with the bar.
if CommandLine.arguments.contains("--selftest") {
    let path = TDClient.resolvePath() ?? "<not found>"
    FileHandle.standardError.write("td path: \(path)\n".data(using: .utf8)!)
    do {
        let r = try TDClient.fetchReport()
        print("OK  schema=\(r.schema) td=\(r.tdVersion)")
        print("    spend  today=\(Money.precise(r.spend.today)) month=\(Money.precise(r.spend.month)) lifetime=\(Money.precise(r.spend.lifetime)) available=\(r.spend.available)")
        print("    saved  today=\(Money.micro(r.saved.today)) lifetime=\(Money.micro(r.saved.lifetime)) share=\(String(format: "%.1f%%", r.sharePct))")
        exit(0)
    } catch {
        FileHandle.standardError.write("selftest failed: \(error)\n".data(using: .utf8)!)
        exit(1)
    }
}

// Menu-bar-only agent: `.accessory` hides the Dock icon and skips the main
// menu, so no Info.plist (LSUIElement) is needed to run unbundled via
// `swift run`. The packaged .app sets LSUIElement too for a clean launch.
let app = NSApplication.shared
app.setActivationPolicy(.accessory)

let delegate = AppDelegate()
app.delegate = delegate
app.run()

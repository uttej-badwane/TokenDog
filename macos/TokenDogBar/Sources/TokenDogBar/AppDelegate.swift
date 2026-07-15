import AppKit

/// The whole UI: an icon-only NSStatusItem (no figure in the bar) with a
/// dropdown that breaks down Claude spend — daily, month-to-date, all-time
/// (from local logs), today's per-model split and tokens — plus whether
/// TokenDog is actually capturing traffic. State refreshes on a timer and
/// whenever the menu opens.
final class AppDelegate: NSObject, NSApplicationDelegate, NSMenuDelegate {
    private var statusItem: NSStatusItem!
    private let menu = NSMenu()
    private var timer: Timer?

    private var lastReport: SpendReport?
    private var lastError: String?

    // Harness is fetched alongside spend but tracked separately: a harness
    // failure (e.g. an older `td` without the command) must never blank the
    // spend UI, so it renders nothing rather than surfacing an error row.
    private var lastHarness: HarnessReport?

    private let refreshInterval: TimeInterval = 60

    func applicationDidFinishLaunching(_ notification: Notification) {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        if let button = statusItem.button {
            button.image = Self.barIcon()
            button.imagePosition = .imageOnly        // icon only — no dollar figure in the bar
            button.toolTip = "TokenDog — Claude spend"
        }
        menu.delegate = self
        statusItem.menu = menu

        rebuildMenu()
        refresh()

        timer = Timer.scheduledTimer(withTimeInterval: refreshInterval, repeats: true) { [weak self] _ in
            self?.refresh()
        }
    }

    /// The circular token badge from the brand kit, sized for the bar.
    private static func barIcon() -> NSImage? {
        guard let img = NSImage(named: "MenuBarIcon") else { return nil }
        img.isTemplate = false
        img.size = NSSize(width: 18, height: 18)
        return img
    }

    // MARK: - Data

    @objc func refresh() {
        DispatchQueue.global(qos: .utility).async { [weak self] in
            guard let self else { return }
            // Harness is best-effort and independent of spend: capture it
            // (or nil on any failure) without disturbing the spend result.
            let harness = try? TDClient.fetchHarness()
            do {
                let report = try TDClient.fetchReport()
                DispatchQueue.main.async {
                    self.lastReport = report
                    self.lastError = nil
                    self.lastHarness = harness
                    self.rebuildMenu()
                }
            } catch {
                DispatchQueue.main.async {
                    self.lastError = "\(error)"
                    self.lastHarness = harness
                    self.rebuildMenu()
                }
            }
        }
    }

    // MARK: - Menu

    func menuWillOpen(_ menu: NSMenu) {
        refresh()
    }

    private func rebuildMenu() {
        menu.removeAllItems()

        if let r = lastReport {
            if r.spend.available {
                buildSpendSections(r)
            } else {
                addInfo("No Claude usage logs found", "")
                addFootnote("Looked in ~/.claude/projects")
            }
            buildHarnessSection()
            menu.addItem(.separator())
            addFootnote("Updated \(Ago.string(r.generatedAt))  ·  td \(r.tdVersion)")
        } else if let err = lastError {
            addInfo(friendlyError(err), "")
            if err.contains("not found") {
                addFootnote("brew install uttej-badwane/tokendog/tokendog")
            }
        } else {
            addInfo("Loading…", "")
        }

        menu.addItem(.separator())
        addAction("Refresh now", #selector(refresh), key: "r")
        addAction("Open full report…", #selector(openReport), key: "o")
        addAction("Open Code Harness…", #selector(openHarness), key: "h")

        let login = NSMenuItem(title: "Launch at login", action: #selector(toggleLogin), keyEquivalent: "")
        login.target = self
        login.state = LoginItem.isEnabled ? .on : .off
        menu.addItem(login)

        menu.addItem(.separator())
        addAction("Quit TokenDog Bar", #selector(quit), key: "q")
    }

    /// Spend + savings detail. Uses schema-2 fields (daily / per-model / tokens /
    /// earliest) when the installed `td` provides them, and degrades gracefully
    /// to a today/month/all-time summary on an older `td`.
    private func buildSpendSections(_ r: SpendReport) {
        // Is TokenDog actually in the request path? Spend comes from Claude's
        // logs; savings only from traffic that passed through TD. Spending today
        // with ~zero savings means TD isn't intercepting.
        if r.spend.today > 0.05 && r.saved.today < 0.005 {
            addStatus("Not capturing today’s traffic", warn: true)
            addFootnote("run `td setup` to route Claude through TokenDog")
            menu.addItem(.separator())
        } else if r.saved.today > 0 {
            addStatus("Capturing via TokenDog", warn: false)
            menu.addItem(.separator())
        }

        addHeader("Spend")
        if let days = r.spend.daily, !days.isEmpty {
            for (i, d) in days.enumerated() where i < 2 || d.usd > 0 {
                addInfo(d.label, Money.precise(d.usd))
            }
        } else {
            addInfo("Today", Money.precise(r.spend.today))
        }
        addInfo("Month to date", Money.precise(r.spend.month))
        addInfo("All-time", Money.precise(r.spend.lifetime))
        if let e = r.spend.earliestLabel, !e.isEmpty {
            addFootnote("all-time = local logs since \(e)")
        } else {
            addFootnote("from local Claude usage logs")
        }

        if let models = r.spend.byModelToday, !models.isEmpty {
            menu.addItem(.separator())
            addHeader("Today by Model")
            for m in models { addInfo(m.model, Money.precise(m.usd)) }
        }
        if let t = r.spend.tokensToday, t.total > 0 {
            addInfo("Tokens", Count.compact(t.total))
            addFootnote("cache \(Count.compact(t.cacheRead)) · out \(Count.compact(t.output)) · in \(Count.compact(t.input))")
        }

        menu.addItem(.separator())
        addHeader("TokenDog Savings")
        addInfo("Saved today", Money.micro(r.saved.today))
        addInfo("Saved all-time", Money.micro(r.saved.lifetime))
        if r.sharePct > 0 {
            addInfo("Share of bill", String(format: "%.1f%%", r.sharePct))
        }
    }

    /// Code Harness summary: a health line plus the top few findings from
    /// `td harness`. Renders nothing when the installed `td` is too old to
    /// provide the command (lastHarness stays nil) so the section simply
    /// disappears rather than showing an error.
    private func buildHarnessSection() {
        guard let h = lastHarness else { return }
        menu.addItem(.separator())
        addHeader("Code Harness")

        if h.isClean {
            addStatus("Setup looks healthy", warn: false)
        } else {
            addStatus(h.headline, warn: h.summary.critical > 0)
            // Findings arrive severity-sorted from td; show the top few.
            for f in h.findings.prefix(3) {
                addInfo(f.shortFile, f.glyph)
                addFootnote(f.issue)
            }
            if h.summary.autoFixable > 0 {
                addFootnote("\(h.summary.autoFixable) auto-fixable · run `td harness apply`")
            }
        }
    }

    // MARK: - Menu item builders
    //
    // Info rows are rendered as custom views, not item titles. AppKit dims the
    // (attributed)Title of a disabled menu item regardless of the colour set,
    // which is what made the dropdown hard to read; an NSTextField inside a view
    // renders at exactly the colour we give it.

    private static let rowWidth: CGFloat = 320
    private static let leftInset: CGFloat = 16
    private static let valueX: CGFloat = 184

    /// A two-column row: label on the left, a bold monospaced value aligned in a
    /// fixed column on the right. Both at full contrast.
    private func addInfo(_ label: String, _ value: String) {
        let row = NSView(frame: NSRect(x: 0, y: 0, width: Self.rowWidth, height: 22))
        let lbl = Self.field(label, font: .systemFont(ofSize: 13), color: .labelColor)
        lbl.frame = NSRect(x: Self.leftInset, y: 3, width: Self.valueX - Self.leftInset - 8, height: 16)
        row.addSubview(lbl)
        if !value.isEmpty {
            let val = Self.field(value, font: .monospacedDigitSystemFont(ofSize: 13, weight: .semibold), color: .labelColor)
            val.frame = NSRect(x: Self.valueX, y: 3, width: Self.rowWidth - Self.valueX - 14, height: 16)
            row.addSubview(val)
        }
        addViewItem(row)
    }

    /// A small uppercase section label in the brand accent (Terracotta) — the
    /// same treatment the website gives its section kickers.
    private func addHeader(_ text: String) {
        addTextRow(text, font: .systemFont(ofSize: 11, weight: .semibold),
                   color: .tdTerracotta, height: 24, topPad: 6)
    }

    /// A small, quieter note line.
    private func addFootnote(_ text: String) {
        addTextRow(text, font: .systemFont(ofSize: 11), color: .secondaryLabelColor)
    }

    /// Capture-status row: a Terracotta dot, with the text accented in Terracotta
    /// when it's a warning (not capturing) and neutral Ink/Bone when all's well.
    private func addStatus(_ text: String, warn: Bool) {
        let row = NSView(frame: NSRect(x: 0, y: 0, width: Self.rowWidth, height: 22))
        let dot = Self.field("●", font: .systemFont(ofSize: 11), color: .tdTerracotta)
        dot.frame = NSRect(x: Self.leftInset, y: 4, width: 14, height: 14)
        row.addSubview(dot)
        let f = Self.field(text, font: .systemFont(ofSize: 12, weight: warn ? .semibold : .regular),
                           color: warn ? .tdTerracotta : .labelColor)
        f.frame = NSRect(x: Self.leftInset + 17, y: 3, width: Self.rowWidth - Self.leftInset - 30, height: 16)
        row.addSubview(f)
        addViewItem(row)
    }

    /// A single full-width text row (header / footnote / status).
    private func addTextRow(_ s: String, font: NSFont, color: NSColor, height: CGFloat = 20, topPad: CGFloat = 0) {
        let row = NSView(frame: NSRect(x: 0, y: 0, width: Self.rowWidth, height: height))
        let f = Self.field(s, font: font, color: color)
        f.frame = NSRect(x: Self.leftInset, y: (height - 16) / 2 - topPad / 2,
                         width: Self.rowWidth - Self.leftInset - 12, height: 16)
        row.addSubview(f)
        addViewItem(row)
    }

    /// A non-editable, transparent label field.
    private static func field(_ s: String, font: NSFont, color: NSColor) -> NSTextField {
        let f = NSTextField(labelWithString: s)
        f.font = font
        f.textColor = color
        f.lineBreakMode = .byTruncatingTail
        return f
    }

    private func addViewItem(_ view: NSView) {
        let item = NSMenuItem()
        item.view = view
        menu.addItem(item)
    }

    private func addAction(_ title: String, _ selector: Selector, key: String) {
        let item = NSMenuItem(title: title, action: selector, keyEquivalent: key)
        item.target = self
        menu.addItem(item)
    }

    private func friendlyError(_ err: String) -> String {
        if err.contains("not found") { return "TokenDog (td) not found on PATH" }
        return "Couldn't read spend: \(err)"
    }

    // MARK: - Actions

    @objc private func openReport() { TDClient.openFullReport() }

    @objc private func openHarness() { TDClient.openHarnessReport() }

    @objc private func toggleLogin() {
        LoginItem.toggle()
        rebuildMenu()
    }

    @objc private func quit() { NSApp.terminate(nil) }
}

// TokenDog brand palette. Terracotta is the brand fill / accent; Clay its
// pressed/deeper variant. Body text uses the system label colours, which map to
// near-Ink on light and near-Bone on dark — the brand's own neutrals.
extension NSColor {
    static let tdTerracotta = NSColor(srgbRed: 193 / 255.0, green: 94 / 255.0, blue: 60 / 255.0, alpha: 1)
    static let tdClay = NSColor(srgbRed: 169 / 255.0, green: 78 / 255.0, blue: 48 / 255.0, alpha: 1)
}

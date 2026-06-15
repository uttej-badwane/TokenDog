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
            do {
                let report = try TDClient.fetchReport()
                DispatchQueue.main.async {
                    self.lastReport = report
                    self.lastError = nil
                    self.rebuildMenu()
                }
            } catch {
                DispatchQueue.main.async {
                    self.lastError = "\(error)"
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
            addStatus("Not capturing today’s traffic", color: .systemOrange)
            addFootnote("run `td setup` to route Claude through TokenDog")
            menu.addItem(.separator())
        } else if r.saved.today > 0 {
            addStatus("Capturing via TokenDog", color: .systemGreen)
            menu.addItem(.separator())
        }

        addHeader("SPEND")
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
            addHeader("TODAY BY MODEL")
            for m in models { addInfo(m.model, Money.precise(m.usd)) }
        }
        if let t = r.spend.tokensToday, t.total > 0 {
            addInfo("Tokens", Count.compact(t.total))
            addFootnote("cache \(Count.compact(t.cacheRead)) · out \(Count.compact(t.output)) · in \(Count.compact(t.input))")
        }

        menu.addItem(.separator())
        addHeader("TOKENDOG SAVINGS")
        addInfo("Saved today", Money.micro(r.saved.today))
        addInfo("Saved all-time", Money.micro(r.saved.lifetime))
        if r.sharePct > 0 {
            addInfo("Share of bill", String(format: "%.1f%%", r.sharePct))
        }
    }

    // MARK: - Menu item builders

    /// Tab stop where the value column begins. Wide enough for the longest label.
    private static let valueColumn: CGFloat = 168

    /// A non-interactive two-column row: a medium-weight label and a bold,
    /// monospaced-digit value aligned in a column. Rendered via attributedTitle
    /// with explicit colours so it reads at full contrast instead of the dim
    /// disabled-grey AppKit applies to plain disabled items.
    private func addInfo(_ label: String, _ value: String) {
        let item = NSMenuItem(title: "", action: nil, keyEquivalent: "")
        item.isEnabled = false

        let para = NSMutableParagraphStyle()
        para.tabStops = [NSTextTab(textAlignment: .left, location: Self.valueColumn)]

        let s = NSMutableAttributedString(string: label, attributes: [
            .font: NSFont.systemFont(ofSize: 13, weight: .regular),
            .foregroundColor: NSColor.secondaryLabelColor,
            .paragraphStyle: para,
        ])
        if !value.isEmpty {
            s.append(NSAttributedString(string: "\t" + value, attributes: [
                .font: NSFont.monospacedDigitSystemFont(ofSize: 13, weight: .semibold),
                .foregroundColor: NSColor.labelColor,
                .paragraphStyle: para,
            ]))
        }
        item.attributedTitle = s
        menu.addItem(item)
    }

    /// A small uppercase section label — readable, not faint.
    private func addHeader(_ text: String) {
        let item = NSMenuItem(title: text, action: nil, keyEquivalent: "")
        item.isEnabled = false
        item.attributedTitle = NSAttributedString(string: text, attributes: [
            .font: NSFont.systemFont(ofSize: 11, weight: .semibold),
            .foregroundColor: NSColor.secondaryLabelColor,
            .kern: 0.8,
        ])
        menu.addItem(item)
    }

    /// A small dimmed note line.
    private func addFootnote(_ text: String) {
        let item = NSMenuItem(title: text, action: nil, keyEquivalent: "")
        item.isEnabled = false
        item.attributedTitle = NSAttributedString(string: text, attributes: [
            .font: NSFont.systemFont(ofSize: 11),
            .foregroundColor: NSColor.secondaryLabelColor,
        ])
        menu.addItem(item)
    }

    /// A coloured status line (capture state) with a leading dot.
    private func addStatus(_ text: String, color: NSColor) {
        let item = NSMenuItem(title: text, action: nil, keyEquivalent: "")
        item.isEnabled = false
        item.attributedTitle = NSAttributedString(string: "● \(text)", attributes: [
            .font: NSFont.systemFont(ofSize: 12, weight: .medium),
            .foregroundColor: color,
        ])
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

    @objc private func toggleLogin() {
        LoginItem.toggle()
        rebuildMenu()
    }

    @objc private func quit() { NSApp.terminate(nil) }
}

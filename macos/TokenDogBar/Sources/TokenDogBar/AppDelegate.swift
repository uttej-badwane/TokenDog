import AppKit

/// The whole UI: a single NSStatusItem whose title shows Claude spend, and a
/// dropdown menu with the breakdown. State is refreshed on a timer and whenever
/// the menu is opened.
final class AppDelegate: NSObject, NSApplicationDelegate, NSMenuDelegate {
    private var statusItem: NSStatusItem!
    private let menu = NSMenu()
    private var timer: Timer?

    /// Last successful report, kept so opening the menu shows data immediately
    /// while a fresh fetch runs in the background.
    private var lastReport: SpendReport?
    private var lastError: String?

    private let refreshInterval: TimeInterval = 60

    func applicationDidFinishLaunching(_ notification: Notification) {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            button.image = Self.barIcon()
            button.imagePosition = .imageLeft
            button.imageHugsTitle = true
            button.title = " …"
        }
        menu.delegate = self
        statusItem.menu = menu

        rebuildMenu()
        refresh()

        timer = Timer.scheduledTimer(withTimeInterval: refreshInterval, repeats: true) { [weak self] _ in
            self?.refresh()
        }
    }

    /// Loads the bundled TokenDog mark, sized for the status bar. Rendered in
    /// full colour (not a template image) — the coin is the brand and reads
    /// well at this size, where a flat monochrome silhouette would not.
    private static func barIcon() -> NSImage? {
        guard let path = Bundle.main.path(forResource: "MenuBarIcon", ofType: "png"),
              let img = NSImage(contentsOfFile: path) else { return nil }
        img.size = NSSize(width: 18, height: 18)
        img.isTemplate = false
        return img
    }

    // MARK: - Data

    /// Fetches a fresh report off the main thread, then updates the UI on it.
    @objc func refresh() {
        DispatchQueue.global(qos: .utility).async { [weak self] in
            guard let self else { return }
            do {
                let report = try TDClient.fetchReport()
                DispatchQueue.main.async {
                    self.lastReport = report
                    self.lastError = nil
                    self.render()
                }
            } catch {
                DispatchQueue.main.async {
                    self.lastError = "\(error)"
                    self.render()
                }
            }
        }
    }

    /// Updates the title and rebuilds the menu from current state.
    private func render() {
        statusItem.button?.title = titleText()
        rebuildMenu()
    }

    /// The always-visible bar text. Prefers today's spend; degrades to lifetime
    /// savings when no Claude logs exist, and to a plain marker when td is gone.
    private func titleText() -> String {
        if lastReport == nil && lastError != nil {
            return " td?"
        }
        guard let r = lastReport else { return " …" }
        if r.spend.available {
            return " \(Money.short(r.spend.today)) today"
        }
        if r.saved.lifetime > 0 {
            return " \(Money.short(r.saved.lifetime)) saved"
        }
        return " td"
    }

    // MARK: - Menu

    func menuWillOpen(_ menu: NSMenu) {
        // Refresh on open so the numbers are current when the user looks.
        refresh()
    }

    private func rebuildMenu() {
        menu.removeAllItems()

        if let r = lastReport {
            if r.spend.available {
                addInfo("Spent today", Money.precise(r.spend.today))
                addInfo("This month", Money.precise(r.spend.month))
                addInfo("Lifetime", Money.precise(r.spend.lifetime))
                addInfo("(via Claude usage logs)", "")
            } else {
                addInfo("No Claude usage logs found", "")
            }
            menu.addItem(.separator())
            addInfo("TD saved today", Money.micro(r.saved.today))
            addInfo("TD saved lifetime", Money.micro(r.saved.lifetime))
            if r.sharePct > 0 {
                addInfo("TD share of bill", String(format: "%.1f%%", r.sharePct))
            }
        } else if let err = lastError {
            addInfo(friendlyError(err), "")
            if err.contains("not found") {
                addInfo("Install: brew install uttej-badwane/tokendog/tokendog", "")
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

    /// A disabled, two-column info row ("Label    value") rendered with a
    /// tab stop so values line up.
    private func addInfo(_ label: String, _ value: String) {
        let title = value.isEmpty ? label : "\(label)\t\(value)"
        let item = NSMenuItem(title: title, action: nil, keyEquivalent: "")
        item.isEnabled = false
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

    @objc private func openReport() {
        TDClient.openFullReport()
    }

    @objc private func toggleLogin() {
        LoginItem.toggle()
        rebuildMenu()
    }

    @objc private func quit() {
        NSApp.terminate(nil)
    }
}

import Foundation
import ServiceManagement

/// Thin wrapper over SMAppService for "Launch at login". Available on macOS 13+
/// with no helper bundle required — the agent registers itself. When the app is
/// run unbundled (e.g. `swift run` during development) registration is a no-op
/// that reports `false`, so the menu item simply stays unchecked.
enum LoginItem {
    static var isEnabled: Bool {
        guard #available(macOS 13.0, *) else { return false }
        return SMAppService.mainApp.status == .enabled
    }

    /// Returns the new state (best-effort). Failures (e.g. running unbundled)
    /// are swallowed and the previous state is returned.
    @discardableResult
    static func toggle() -> Bool {
        guard #available(macOS 13.0, *) else { return false }
        do {
            if SMAppService.mainApp.status == .enabled {
                try SMAppService.mainApp.unregister()
                return false
            } else {
                try SMAppService.mainApp.register()
                return true
            }
        } catch {
            return isEnabled
        }
    }
}

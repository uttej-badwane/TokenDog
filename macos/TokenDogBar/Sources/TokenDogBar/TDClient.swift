import Foundation

/// Locates and runs the `td` CLI. A menu-bar agent inherits a minimal PATH
/// (launchd doesn't source your shell rc), so we probe the usual install
/// locations explicitly in addition to whatever PATH we were handed.
enum TDClient {
    enum Failure: Error, CustomStringConvertible {
        case notFound
        case ranButFailed(code: Int32, stderr: String)
        case badOutput(String)

        var description: String {
            switch self {
            case .notFound:
                return "td not found"
            case .ranButFailed(let code, let stderr):
                return "td exited \(code): \(stderr)"
            case .badOutput(let msg):
                return "couldn't read td output: \(msg)"
            }
        }
    }

    /// Common Homebrew / manual install locations, tried in order after PATH.
    private static let candidates = [
        "/opt/homebrew/bin/td", // Apple Silicon Homebrew
        "/usr/local/bin/td",    // Intel Homebrew / manual
        "\(NSHomeDirectory())/go/bin/td",
    ]

    /// Resolves an absolute path to `td`, or nil if it can't be found.
    static func resolvePath() -> String? {
        // 1) Honour an explicit override (useful for dev / packaging).
        if let override = ProcessInfo.processInfo.environment["TOKENDOG_BIN"],
           FileManager.default.isExecutableFile(atPath: override) {
            return override
        }
        // 2) Anything already on PATH.
        if let onPath = which("td") {
            return onPath
        }
        // 3) Known install locations.
        for c in candidates where FileManager.default.isExecutableFile(atPath: c) {
            return c
        }
        return nil
    }

    /// Runs `td spend --json` and decodes the report. Synchronous; callers run
    /// it off the main thread.
    static func fetchReport() throws -> SpendReport {
        guard let path = resolvePath() else { throw Failure.notFound }

        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: path)
        proc.arguments = ["spend", "--json"]

        let out = Pipe()
        let err = Pipe()
        proc.standardOutput = out
        proc.standardError = err

        try proc.run()
        let outData = out.fileHandleForReading.readDataToEndOfFile()
        let errData = err.fileHandleForReading.readDataToEndOfFile()
        proc.waitUntilExit()

        if proc.terminationStatus != 0 {
            let stderr = String(data: errData, encoding: .utf8) ?? ""
            throw Failure.ranButFailed(code: proc.terminationStatus, stderr: stderr.trimmingCharacters(in: .whitespacesAndNewlines))
        }
        do {
            return try SpendReport.decode(from: outData)
        } catch {
            throw Failure.badOutput(error.localizedDescription)
        }
    }

    /// Opens the full breakdown in Terminal (keeps the window up with `td gain`).
    static func openFullReport() {
        guard let path = resolvePath() else { return }
        let script = "tell application \"Terminal\" to do script \"\(path) gain --by-model\"\n" +
                     "tell application \"Terminal\" to activate"
        if let appleScript = NSAppleScript(source: script) {
            var errInfo: NSDictionary?
            appleScript.executeAndReturnError(&errInfo)
        }
    }

    /// `which`-style PATH lookup without spawning a shell.
    private static func which(_ name: String) -> String? {
        let path = ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin"
        for dir in path.split(separator: ":") {
            let candidate = "\(dir)/\(name)"
            if FileManager.default.isExecutableFile(atPath: candidate) {
                return candidate
            }
        }
        return nil
    }
}

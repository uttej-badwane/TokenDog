import Foundation

/// Mirrors the JSON contract emitted by `td spend --json` (internal/spend.Report).
/// Decoded with `.convertFromSnakeCase`, so Swift property names are camelCase.
///
/// Fields added in schema 2 (per-model split, today's tokens, earliest-log
/// label) are optional so this client still decodes a schema-1 `td` cleanly —
/// it just renders fewer rows until `td` is upgraded.
struct SpendReport: Decodable {
    struct ModelSpend: Decodable {
        let model: String
        let usd: Double
    }

    struct Tokens: Decodable {
        let input: Int
        let output: Int
        let cacheRead: Int
        let cacheCreation: Int
        var total: Int { input + output + cacheRead + cacheCreation }
    }

    struct DaySpend: Decodable {
        let date: String
        let label: String
        let usd: Double
    }

    struct Spend: Decodable {
        let today: Double
        let month: Double
        let lifetime: Double
        let currency: String
        let available: Bool
        // schema 2 (optional)
        let earliestLabel: String?
        let byModelToday: [ModelSpend]?
        let tokensToday: Tokens?
        let daily: [DaySpend]?
    }

    struct Saved: Decodable {
        let today: Double
        let lifetime: Double
        let tokens: Int
    }

    let schema: Int
    let generatedAt: Date
    let spend: Spend
    let saved: Saved
    let sharePct: Double
    let tdVersion: String

    /// The highest schema version this client renders in full. A newer `td`
    /// still decodes (unknown keys are ignored); an older one omits the schema-2
    /// fields, which are optional.
    static let supportedSchema = 2

    static func decode(from data: Data) throws -> SpendReport {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(SpendReport.self, from: data)
    }
}

/// Formats USD amounts. Cents in the dropdown; sub-cent savings get more digits.
enum Money {
    static func precise(_ v: Double) -> String { String(format: "$%.2f", v) }

    static func micro(_ v: Double) -> String {
        if v > 0 && v < 0.01 { return String(format: "$%.4f", v) }
        return String(format: "$%.2f", v)
    }
}

/// Compact token counts: 1_234_567 → "1.2M", 12_300 → "12.3K".
enum Count {
    static func compact(_ n: Int) -> String {
        let v = Double(n)
        if v >= 1_000_000 { return String(format: "%.1fM", v / 1_000_000) }
        if v >= 1_000 { return String(format: "%.1fK", v / 1_000) }
        return String(n)
    }
}

/// "just now" / "3m ago" / "1h ago" for the last-refresh line.
enum Ago {
    static func string(_ date: Date) -> String {
        let s = Int(max(0, Date().timeIntervalSince(date)))
        if s < 10 { return "just now" }
        if s < 60 { return "\(s)s ago" }
        if s < 3600 { return "\(s / 60)m ago" }
        return "\(s / 3600)h ago"
    }
}

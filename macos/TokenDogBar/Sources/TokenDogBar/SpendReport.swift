import Foundation

/// Mirrors the JSON contract emitted by `td spend --json` (internal/spend.Report).
/// Decoded with `.convertFromSnakeCase`, so Swift property names are camelCase.
struct SpendReport: Decodable {
    struct Spend: Decodable {
        let today: Double
        let month: Double
        let lifetime: Double
        let currency: String
        let available: Bool
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

    /// The highest schema version this client knows how to render. If a future
    /// `td` bumps the schema, we still decode the fields we recognise but can
    /// surface a hint to upgrade the app.
    static let supportedSchema = 1

    static func decode(from data: Data) throws -> SpendReport {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(SpendReport.self, from: data)
    }
}

/// Formats a USD amount the way the menu bar wants it: whole-dollar precision
/// in the always-visible title, cents in the dropdown.
enum Money {
    static func short(_ v: Double) -> String {
        if v >= 100 { return String(format: "$%.0f", v) }
        return String(format: "$%.2f", v)
    }

    static func precise(_ v: Double) -> String {
        return String(format: "$%.2f", v)
    }

    static func micro(_ v: Double) -> String {
        // Savings are often sub-cent; show enough digits to be non-zero.
        if v > 0 && v < 0.01 { return String(format: "$%.4f", v) }
        return String(format: "$%.2f", v)
    }
}

import Foundation

/// Mirrors the JSON contract emitted by `td harness --json`
/// (internal/harness.Report). Decoded with `.convertFromSnakeCase`, so
/// Swift property names are camelCase.
///
/// Only the fields the menu bar renders are modelled; `inventory` and
/// per-finding `fix_id` are ignored (the decoder drops unknown keys).
/// Any field the current `td` might not emit yet is Optional so an older
/// binary still decodes cleanly.
struct HarnessReport: Decodable {
    struct Summary: Decodable {
        let critical: Int
        let warning: Int
        let info: Int
        let autoFixable: Int
        let filesScanned: Int
    }

    struct Finding: Decodable {
        let file: String
        let issue: String
        let severity: String
        let fix: String
        let scope: String
        let dimension: String
        let autoFixable: Bool
    }

    let schema: Int
    let generatedAt: Date
    let tdVersion: String
    let projectRoot: String?
    let summary: Summary
    let findings: [Finding]

    /// The highest schema version this client renders in full. A newer
    /// `td` still decodes (unknown keys ignored); this client just tracks
    /// what it was written against.
    static let supportedSchema = 1

    /// True when the audit surfaced nothing at all.
    var isClean: Bool {
        summary.critical == 0 && summary.warning == 0 && summary.info == 0
    }

    /// A compact "2 critical · 5 warnings" line, omitting empty buckets.
    var headline: String {
        var parts: [String] = []
        if summary.critical > 0 { parts.append("\(summary.critical) critical") }
        if summary.warning > 0 { parts.append("\(summary.warning) \(summary.warning == 1 ? "warning" : "warnings")") }
        if summary.info > 0 { parts.append("\(summary.info) info") }
        return parts.isEmpty ? "Setup looks healthy" : parts.joined(separator: " · ")
    }

    static func decode(from data: Data) throws -> HarnessReport {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(HarnessReport.self, from: data)
    }
}

extension HarnessReport.Finding {
    /// The last path segment, for a compact left-column label.
    var shortFile: String {
        (file as NSString).lastPathComponent
    }

    /// A single-glyph severity marker for the value column.
    var glyph: String {
        switch severity {
        case "critical": return "●"
        case "warning": return "▲"
        default: return "·"
        }
    }
}

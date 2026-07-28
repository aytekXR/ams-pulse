import Foundation

// MARK: - PaginatedMeta

/// Pagination envelope present in all list responses.
/// Spec reference: lines 1872-1880 (`PaginatedMeta` schema).
///
/// Both fields are optional:
/// - `nextCursor`: opaque cursor for the next page; nil/null means last page
/// - `total`: total count if cheaply computable; omitted otherwise
public struct PaginatedMeta: Decodable, Sendable, Equatable, Hashable {
    /// Opaque cursor for the next page; nil if this is the last page.
    public let nextCursor: String?

    /// Total count if cheaply computable; nil otherwise.
    public let total: Int?

    enum CodingKeys: String, CodingKey {
        case nextCursor = "next_cursor"
        case total
    }

    public init(nextCursor: String? = nil, total: Int? = nil) {
        self.nextCursor = nextCursor
        self.total = total
    }
}

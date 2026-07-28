import Foundation

// MARK: - LoadingState

/// Generic loading state for view models.
public enum LoadingState<T: Sendable>: Sendable {
    case idle
    case loading
    case loaded(T)
    case failed(PulseAPIError)
}

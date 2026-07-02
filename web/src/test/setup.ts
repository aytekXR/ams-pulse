import "@testing-library/jest-dom";
import { beforeAll, afterEach, afterAll } from "vitest";
import { server } from "./mocks/server";

// Start the msw server before all tests; reset per-test overrides after each
// test; shut down cleanly after the suite.  Tests that use vi.mock("@/api/client")
// are unaffected — their module replacement means fetch is never called.
beforeAll(() => server.listen({ onUnhandledRequest: "warn" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

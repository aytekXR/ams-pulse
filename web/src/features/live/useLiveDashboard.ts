import { useState, useEffect, useRef, useCallback } from "react";
import { LiveSocket, liveApi, ApiError } from "@/api/client";
import type { LiveOverview, LiveStream } from "@/lib/api/types";

interface UseLiveDashboardResult {
  overview: LiveOverview | null;
  streams: LiveStream[];
  connected: boolean;
  loading: boolean;
  error: string | null;
  refresh: () => void;
}

const POLL_INTERVAL_MS = 5_000;

export function useLiveDashboard(): UseLiveDashboardResult {
  const [overview, setOverview] = useState<LiveOverview | null>(null);
  const [streams, setStreams] = useState<LiveStream[]>([]);
  const [connected, setConnected] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const socketRef = useRef<LiveSocket | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const mountedRef = useRef(true);

  const fetchRest = useCallback(async () => {
    try {
      const [ov, sl] = await Promise.all([
        liveApi.getOverview(),
        liveApi.getStreams({ limit: 500 }),
      ]);
      if (!mountedRef.current) return;
      setOverview(ov);
      // LiveStreamList uses `items` per generated schema
      setStreams(sl.items ?? []);
      setError(null);
    } catch (err) {
      if (!mountedRef.current) return;
      const msg = err instanceof ApiError ? err.message : "Failed to load live data";
      setError(msg);
    } finally {
      if (mountedRef.current) setLoading(false);
    }
  }, []);

  const refresh = useCallback(() => {
    setLoading(true);
    void fetchRest();
  }, [fetchRest]);

  useEffect(() => {
    mountedRef.current = true;

    // Initial REST fetch regardless of WS
    void fetchRest();

    // Spin up WebSocket
    const sock = new LiveSocket({
      onStatusChange: (isConnected) => {
        if (!mountedRef.current) return;
        setConnected(isConnected);
        if (isConnected) {
          // WS up — clear polling interval
          if (pollRef.current !== null) {
            clearInterval(pollRef.current);
            pollRef.current = null;
          }
        } else {
          // WS down — fall back to polling
          if (pollRef.current === null) {
            pollRef.current = setInterval(() => {
              void fetchRest();
            }, POLL_INTERVAL_MS);
          }
        }
      },
    });

    sock.subscribe((msg) => {
      if (!mountedRef.current) return;
      if (msg.type === "snapshot" || msg.type === "delta") {
        const payload = msg.payload;
        if (payload) {
          setOverview((prev) =>
            prev ? { ...prev, ...payload } : (payload as LiveOverview),
          );
          // LiveOverview does not carry the full stream list; stream list is
          // always fetched via REST. WS updates the overview stats only.
          setLoading(false);
          setError(null);
        }
      }
    });

    sock.connect();
    socketRef.current = sock;

    // Fallback polling starts immediately; cleared when WS connects
    pollRef.current = setInterval(() => {
      void fetchRest();
    }, POLL_INTERVAL_MS);

    return () => {
      mountedRef.current = false;
      sock.destroy();
      socketRef.current = null;
      if (pollRef.current !== null) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
    };
  }, [fetchRest]);

  return { overview, streams, connected, loading, error, refresh };
}

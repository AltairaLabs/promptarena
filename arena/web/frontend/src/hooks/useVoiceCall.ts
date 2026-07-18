import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { MicCapture } from "@/audio/micCapture";
import { WsPlayback } from "@/audio/wsPlayback";
import { VoiceTransport, type CallState } from "@/voice/voiceTransport";

export interface LiveConsoleCall {
  state: CallState;
  muted: boolean;
  level: number;
  errorMessage?: string;
  onCall: () => void;
  onHangup: () => void;
  onToggleMute: () => void;
}

// Injection seam so the hook is unit-testable without real Web Audio / WS.
export const voiceFactories = {
  makeMic: () => new MicCapture(),
  makePlayback: () => new WsPlayback(new AudioContext()),
  makeTransport: (opts: ConstructorParameters<typeof VoiceTransport>[0]) => new VoiceTransport(opts),
};

export function useVoiceCall(opts: { sessionId: string | null; enabled: boolean }): LiveConsoleCall {
  const [state, setState] = useState<CallState>("idle");
  const [muted, setMuted] = useState(false);
  const [level, setLevel] = useState(0);
  const [errorMessage, setErrorMessage] = useState<string | undefined>();

  const micRef = useRef<ReturnType<typeof voiceFactories.makeMic> | null>(null);
  const playbackRef = useRef<ReturnType<typeof voiceFactories.makePlayback> | null>(null);
  const transportRef = useRef<ReturnType<typeof voiceFactories.makeTransport> | null>(null);
  const genRef = useRef(0);

  // Resource-only teardown — no setState, so it's safe to call during unmount.
  const teardownRefs = useCallback(() => {
    genRef.current++; // invalidate in-flight callbacks
    micRef.current?.stop();
    transportRef.current?.close();
    playbackRef.current?.close();
    micRef.current = null;
    transportRef.current = null;
    playbackRef.current = null;
  }, []);

  const onCall = useCallback(() => {
    if (!opts.sessionId || !opts.enabled) return;
    // Tear down any prior/active instances before starting fresh (double-invoke safety + retry-after-error).
    teardownRefs();

    const gen = ++genRef.current;
    setErrorMessage(undefined);

    const url = `${location.origin.replace(/^http/, "ws")}/api/interactive/voice?session=${encodeURIComponent(opts.sessionId)}&bargein=1`;
    const playback = voiceFactories.makePlayback();
    const transport = voiceFactories.makeTransport({
      url,
      playback,
      // Ignore callbacks from a superseded transport; never downgrade error->idle.
      onState: (s) => {
        if (gen === genRef.current) setState((prev) => (prev === "error" && s === "idle" ? prev : s));
      },
      onError: (m) => {
        if (gen === genRef.current) {
          setErrorMessage(m);
          setState("error");
        }
      },
    });
    const mic = voiceFactories.makeMic();
    playbackRef.current = playback;
    transportRef.current = transport;
    micRef.current = mic;

    transport.connect();
    mic
      .start(
        (buf) => transport.sendPcm(buf),
        (v) => {
          if (gen === genRef.current) setLevel(v);
        },
      )
      .catch((e: unknown) => {
        if (gen !== genRef.current) return; // superseded — ignore
        micRef.current?.stop();
        transportRef.current?.close();
        playbackRef.current?.close();
        micRef.current = null;
        transportRef.current = null;
        playbackRef.current = null;
        setErrorMessage(e instanceof Error ? e.message : String(e));
        setState("error"); // guard keeps this from being clobbered by trailing onclose->idle
      });
  }, [opts.sessionId, opts.enabled, teardownRefs]);

  const onHangup = useCallback(() => {
    teardownRefs();
    setState("idle");
    setLevel(0);
    setMuted(false);
    setErrorMessage(undefined);
  }, [teardownRefs]);

  // Unmount cleanup: if the consumer unmounts mid-call (e.g. navigating away),
  // tear down the mic/socket/AudioContext instead of leaking them.
  useEffect(() => teardownRefs, [teardownRefs]);

  const onToggleMute = useCallback(() => {
    setMuted((m) => {
      const next = !m;
      transportRef.current?.setMuted(next);
      return next;
    });
  }, []);

  return useMemo(
    () => ({ state, muted, level, errorMessage, onCall, onHangup, onToggleMute }),
    [state, muted, level, errorMessage, onCall, onHangup, onToggleMute],
  );
}

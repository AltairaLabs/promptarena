import { useCallback, useMemo, useRef, useState } from "react";
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

  const onCall = useCallback(() => {
    if (!opts.sessionId || !opts.enabled) return;
    setErrorMessage(undefined);
    const playback = voiceFactories.makePlayback();
    const url = `${location.origin.replace(/^http/, "ws")}/api/interactive/voice?session=${encodeURIComponent(opts.sessionId)}&bargein=1`;
    const transport = voiceFactories.makeTransport({
      url,
      playback,
      onState: (s) => setState(s),
      onError: (m) => {
        setErrorMessage(m);
        setState("error");
      },
    });
    const mic = voiceFactories.makeMic();
    playbackRef.current = playback;
    transportRef.current = transport;
    micRef.current = mic;

    transport.connect();
    void mic.start(
      (buf) => transport.sendPcm(buf),
      (v) => setLevel(v),
    );
  }, [opts.sessionId, opts.enabled]);

  const onHangup = useCallback(() => {
    micRef.current?.stop();
    transportRef.current?.close();
    playbackRef.current?.close();
    micRef.current = null;
    transportRef.current = null;
    playbackRef.current = null;
    setState("idle");
    setLevel(0);
    setMuted(false);
  }, []);

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

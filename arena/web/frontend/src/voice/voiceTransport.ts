export type CallState = "idle" | "connecting" | "live" | "listening" | "speaking" | "error";

interface PlaybackSink {
  enqueue(b: ArrayBuffer): void;
  flush(): void;
}

interface VoiceTransportOpts {
  url: string;
  playback: PlaybackSink;
  onState: (s: CallState) => void;
  onError: (m: string) => void;
  wsFactory?: (url: string) => WebSocket;
}

// VoiceTransport owns the voice WebSocket: sends binary mic PCM (dropped
// while muted) plus a `mute` control frame, and receives binary playback
// (forwarded to the injected PlaybackSink) and JSON control frames
// (state/flush/error), surfacing progress via onState/onError callbacks.
export class VoiceTransport {
  private ws: WebSocket | null = null;
  private muted = false;

  constructor(private opts: VoiceTransportOpts) {}

  connect(): void {
    this.opts.onState("connecting");
    const factory = this.opts.wsFactory ?? ((u: string) => new WebSocket(u));
    const ws = factory(this.opts.url);
    ws.binaryType = "arraybuffer";
    ws.onopen = () => this.opts.onState("live");
    ws.onclose = () => this.opts.onState("idle");
    ws.onerror = () => this.opts.onError("connection error");
    ws.onmessage = (e: MessageEvent) => this.onMessage(e);
    this.ws = ws;
  }

  private onMessage(e: MessageEvent): void {
    if (typeof e.data === "string") {
      try {
        const msg = JSON.parse(e.data) as { type: string; state?: CallState; message?: string };
        if (msg.type === "state" && msg.state) this.opts.onState(msg.state);
        else if (msg.type === "flush") this.opts.playback.flush();
        else if (msg.type === "error") this.opts.onError(msg.message ?? "voice error");
      } catch {
        /* ignore malformed control frame */
      }
      return;
    }
    this.opts.playback.enqueue(e.data as ArrayBuffer);
  }

  sendPcm(buf: ArrayBuffer): void {
    if (this.muted) return;
    if (this.ws && this.ws.readyState === WebSocket.OPEN) this.ws.send(buf);
  }

  setMuted(m: boolean): void {
    this.muted = m;
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: "mute", muted: m }));
    }
  }

  close(): void {
    this.ws?.close();
    this.ws = null;
  }
}

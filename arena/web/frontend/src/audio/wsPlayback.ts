import { int16ToFloat32 } from "@/audio/player";

const PLAYBACK_RATE = 24000;

// WsPlayback gaplessly schedules PCM16@24k frames on a shared AudioContext,
// mirroring player.ts's running-clock approach. flush() drops queued audio for
// barge-in (server sends {"type":"flush"}).
export class WsPlayback {
  private ctx: AudioContext;
  private nextStart = 0;
  private live: AudioBufferSourceNode[] = [];

  constructor(ctx: AudioContext) {
    this.ctx = ctx;
  }

  enqueue(pcm: ArrayBuffer): void {
    const samples = new Int16Array(pcm);
    if (samples.length === 0) return;
    const buffer = this.ctx.createBuffer(1, samples.length, PLAYBACK_RATE);
    buffer.copyToChannel(int16ToFloat32(samples), 0);

    const node = this.ctx.createBufferSource();
    node.buffer = buffer;
    node.connect(this.ctx.destination);
    node.onended = () => {
      this.live = this.live.filter((n) => n !== node);
    };

    const now = this.ctx.currentTime;
    const startAt = Math.max(now, this.nextStart);
    node.start(startAt);
    this.nextStart = startAt + buffer.duration;
    this.live.push(node);
  }

  flush(): void {
    for (const n of this.live) {
      try {
        n.stop();
      } catch {
        /* already stopped */
      }
    }
    this.live = [];
    this.nextStart = this.ctx.currentTime;
  }

  close(): void {
    this.flush();
    void this.ctx.close();
  }
}

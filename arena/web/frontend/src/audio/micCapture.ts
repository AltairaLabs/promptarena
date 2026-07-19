import { downsampleTo16k, float32ToInt16 } from "./pcm";
import { CAPTURE_WORKLET_SRC } from "./capture-worklet";

export function rms(frame: Float32Array): number {
  if (frame.length === 0) return 0;
  let sum = 0;
  for (let i = 0; i < frame.length; i++) sum += frame[i] * frame[i];
  return Math.sqrt(sum / frame.length);
}

export function frameToPcm16le(frame: Float32Array, inRate: number): ArrayBuffer {
  const down = downsampleTo16k(frame, inRate);
  return float32ToInt16(down).buffer;
}

// MicCapture wires getUserMedia → AudioWorklet → onPcm(16k PCM16 LE) + onLevel(RMS).
// AEC/AGC/noise-suppression are enabled at the constraint level (browser-edge AEC).
export class MicCapture {
  private ctx: AudioContext | null = null;
  private stream: MediaStream | null = null;
  private node: AudioWorkletNode | null = null;

  async start(onPcm: (buf: ArrayBuffer) => void, onLevel: (v: number) => void): Promise<void> {
    this.stream = await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
    });
    this.ctx = new AudioContext();
    const url = URL.createObjectURL(new Blob([CAPTURE_WORKLET_SRC], { type: "text/javascript" }));
    await this.ctx.audioWorklet.addModule(url);
    URL.revokeObjectURL(url);

    const source = this.ctx.createMediaStreamSource(this.stream);
    this.node = new AudioWorkletNode(this.ctx, "capture-processor");
    const inRate = this.ctx.sampleRate;
    this.node.port.onmessage = (ev: MessageEvent<Float32Array>) => {
      const frame = ev.data;
      onLevel(rms(frame));
      onPcm(frameToPcm16le(frame, inRate));
    };
    source.connect(this.node);
    // Do not connect node → destination (would echo mic to speakers).
  }

  stop(): void {
    this.node?.port.close();
    this.node?.disconnect();
    this.stream?.getTracks().forEach((t) => t.stop());
    void this.ctx?.close();
    this.node = null;
    this.stream = null;
    this.ctx = null;
  }
}

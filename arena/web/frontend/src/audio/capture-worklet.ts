// AudioWorklet processor that forwards mono float frames to the main thread.
// Registered at runtime via a Blob URL; kept as a string because worklets load
// from a URL, not an import. The main thread does resampling/encoding.
export const CAPTURE_WORKLET_SRC = `
class CaptureProcessor extends AudioWorkletProcessor {
  process(inputs) {
    const ch = inputs[0] && inputs[0][0];
    if (ch) this.port.postMessage(ch.slice(0));
    return true;
  }
}
registerProcessor('capture-processor', CaptureProcessor);
`;

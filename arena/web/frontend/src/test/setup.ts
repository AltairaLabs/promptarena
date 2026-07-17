import "@testing-library/jest-dom/vitest";

// React Flow measures its container and every node via ResizeObserver, and
// none of that exists in jsdom. Without a stub, mounting <ReactFlow> throws
// "ResizeObserver is not defined" before any assertions run. A true no-op
// stub isn't enough either: React Flow's node internals only get marked
// "measured" — and edges only get drawn — once a resize entry has actually
// been delivered for each node, so `observe()` below delivers one.
class ResizeObserverStub implements ResizeObserver {
  private readonly callback: ResizeObserverCallback;

  constructor(callback: ResizeObserverCallback) {
    this.callback = callback;
  }

  observe(target: Element): void {
    // Deferred (not synchronous): React Flow's own domNode-in-store wiring
    // and this per-node observe() call both fire from `useEffect`s in the
    // same commit, in child-before-parent order — calling back synchronously
    // here would race ahead of the store having a `domNode` to measure
    // against. A real ResizeObserver's callback is asynchronous too, so a
    // macrotask hop matches actual browser timing closely enough for tests.
    const entry = { target, contentRect: target.getBoundingClientRect() } as ResizeObserverEntry;
    setTimeout(() => this.callback([entry], this), 0);
  }

  unobserve(): void {}
  disconnect(): void {}
}

if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;
}

// getDimensions() (the helper React Flow's ResizeObserver callback uses to
// size nodes) reads offsetWidth/offsetHeight, not getBoundingClientRect —
// jsdom hardcodes both to 0 with no layout engine, so nodes would stay
// "unmeasured" forever even with the ResizeObserver stub above.
if (typeof HTMLElement !== "undefined") {
  Object.defineProperty(HTMLElement.prototype, "offsetWidth", { configurable: true, value: 150 });
  Object.defineProperty(HTMLElement.prototype, "offsetHeight", { configurable: true, value: 44 });
}

// jsdom's Element#getBoundingClientRect always returns all-zero dimensions,
// which sends React Flow's viewport-fitting math (and dagre-derived
// transforms) down zero/NaN paths. A fixed non-zero size is enough for a
// mount-and-assert-on-labels smoke test.
if (typeof Element !== "undefined" && !("__workflowGraphViewPatched" in Element.prototype)) {
  Object.defineProperty(Element.prototype, "getBoundingClientRect", {
    configurable: true,
    value: () => ({
      width: 1024,
      height: 768,
      top: 0,
      left: 0,
      bottom: 768,
      right: 1024,
      x: 0,
      y: 0,
      toJSON() {
        return {};
      },
    }),
  });
  Object.defineProperty(Element.prototype, "__workflowGraphViewPatched", { value: true });
}

// Edge labels measure themselves via SVGTextElement#getBBox, which jsdom
// (no real layout/rendering engine) doesn't implement at all.
if (typeof SVGElement !== "undefined" && !("getBBox" in SVGElement.prototype)) {
  Object.defineProperty(SVGElement.prototype, "getBBox", {
    configurable: true,
    value: () => ({ x: 0, y: 0, width: 40, height: 14 }),
  });
}

// React Flow's internal transform math constructs DOMMatrixReadOnly, which
// jsdom doesn't implement.
if (typeof globalThis.DOMMatrixReadOnly === "undefined") {
  class DOMMatrixReadOnlyStub {
    m22 = 1;
    constructor(_init?: string) {}
  }
  globalThis.DOMMatrixReadOnly = DOMMatrixReadOnlyStub as unknown as typeof DOMMatrixReadOnly;
}

// Web Audio / mic stubs for jsdom (voice modules import these at module load).
if (!("AudioContext" in globalThis)) {
  (globalThis as unknown as { AudioContext: unknown }).AudioContext = class {};
}
if (!("AudioWorkletNode" in globalThis)) {
  (globalThis as unknown as { AudioWorkletNode: unknown }).AudioWorkletNode = class {};
}

import { JSDOM } from "jsdom";
export function setupDOM() {
  const dom = new JSDOM(
    "<!doctype html><html><head></head><body><div id='app'></div></body></html>",
    { url: "http://127.0.0.1:7463/", pretendToBeVisual: true },
  );
  const descriptors = new Map();
  const set = (key, value) => {
    descriptors.set(key, Object.getOwnPropertyDescriptor(globalThis, key));
    Object.defineProperty(globalThis, key, {
      configurable: true,
      writable: true,
      value,
    });
  };
  for (const key of [
    "window",
    "Window",
    "document",
    "navigator",
    "location",
    "history",
    "Element",
    "HTMLElement",
    "HTMLInputElement",
    "HTMLTextAreaElement",
    "HTMLButtonElement",
    "HTMLFormElement",
    "HTMLSelectElement",
    "Node",
    "NodeFilter",
    "MutationObserver",
    "CustomEvent",
    "Event",
    "MouseEvent",
    "KeyboardEvent",
    "FocusEvent",
    "getComputedStyle",
  ])
    set(
      key,
      key === "window"
        ? dom.window
        : typeof dom.window[key] === "function" && key === "getComputedStyle"
          ? dom.window[key].bind(dom.window)
          : dom.window[key],
    );
  set("IS_REACT_ACT_ENVIRONMENT", true);
  // jsdom has no layout engine; these tests assert state/requests, not geometry.
  set(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  set("requestAnimationFrame", (callback) =>
    setTimeout(() => callback(Date.now()), 0),
  );
  set("cancelAnimationFrame", (timer) => clearTimeout(timer));
  // Polling is advanced explicitly; unit tests never contact a live service.
  const intervals = new Map();
  let id = 0;
  set("setInterval", (callback) => {
    intervals.set(++id, callback);
    return id;
  });
  set("clearInterval", (id) => intervals.delete(id));
  set("tickIntervals", async () => {
    for (const callback of [...intervals.values()]) await callback();
  });
  return () => {
    dom.window.close();
    for (const [key, descriptor] of descriptors) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else delete globalThis[key];
    }
  };
}

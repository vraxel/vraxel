import "@testing-library/jest-dom/vitest"

// Polyfill localStorage for jsdom (Node 22+ built-in localStorage may conflict)
if (
  typeof globalThis.localStorage === "undefined" ||
  typeof globalThis.localStorage.setItem !== "function"
) {
  const store: Record<string, string> = {}
  globalThis.localStorage = {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => {
      store[key] = value
    },
    removeItem: (key: string) => {
      delete store[key]
    },
    clear: () => {
      for (const key in store) delete store[key]
    },
    get length() {
      return Object.keys(store).length
    },
    key: (index: number) => Object.keys(store)[index] ?? null,
  } as Storage
}

// jsdom implements neither ResizeObserver nor matchMedia. TruncateText /
// TruncateCell observe their own box to decide whether to offer an
// overflow tooltip, so without this stub every test that renders a
// truncating cell throws. A no-op is the right shape: jsdom does no
// layout, so nothing would ever resize.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= ResizeObserverStub as unknown as typeof ResizeObserver

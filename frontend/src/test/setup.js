const values = new Map();

const localStorage = {
  get length() {
    return values.size;
  },
  clear() {
    values.clear();
  },
  getItem(key) {
    return values.get(String(key)) ?? null;
  },
  key(index) {
    return [...values.keys()][index] ?? null;
  },
  removeItem(key) {
    values.delete(String(key));
  },
  setItem(key, value) {
    values.set(String(key), String(value));
  },
};

// Node 25 exposes an incomplete native localStorage unless a backing file is
// configured. Component tests need browser semantics, so provide them up front.
Object.defineProperty(globalThis, 'localStorage', {
  configurable: true,
  value: localStorage,
});

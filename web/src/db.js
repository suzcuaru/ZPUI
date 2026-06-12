const DB_NAME = 'zpui';
const DB_VERSION = 2;
const STORE_SNAP = 'snapshots';
const STORE_CACHE = 'cache';
const TTL_MS = 24 * 60 * 60 * 1000;

let dbPromise = null;

function openDB() {
  if (dbPromise) return dbPromise;
  dbPromise = new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE_SNAP)) {
        const store = db.createObjectStore(STORE_SNAP, { keyPath: 'ts' });
        store.createIndex('ts', 'ts');
      }
      if (!db.objectStoreNames.contains(STORE_CACHE)) {
        db.createObjectStore(STORE_CACHE, { keyPath: 'key' });
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
  return dbPromise;
}

export async function logSnapshot(data) {
  const db = await openDB();
  const tx = db.transaction(STORE_SNAP, 'readwrite');
  tx.objectStore(STORE_SNAP).put({ ts: Date.now(), ...data });
  return new Promise((res, rej) => { tx.oncomplete = res; tx.onerror = () => rej(tx.error); });
}

export async function getSnapshots(sinceMs) {
  const db = await openDB();
  const tx = db.transaction(STORE_SNAP, 'readonly');
  const store = tx.objectStore(STORE_SNAP);
  const range = IDBKeyRange.lowerBound(sinceMs || Date.now() - TTL_MS);
  return new Promise((res, rej) => {
    const req = store.index('ts').getAll(range);
    req.onsuccess = () => res(req.result || []);
    req.onerror = () => rej(req.error);
  });
}

export async function cleanOld() {
  const db = await openDB();
  const tx = db.transaction(STORE_SNAP, 'readwrite');
  const store = tx.objectStore(STORE_SNAP);
  const range = IDBKeyRange.upperBound(Date.now() - TTL_MS);
  store.delete(range);
  return new Promise((res, rej) => { tx.oncomplete = res; tx.onerror = () => rej(tx.error); });
}

export async function cacheGet(key) {
  try {
    const db = await openDB();
    const tx = db.transaction(STORE_CACHE, 'readonly');
    return new Promise((res, rej) => {
      const req = tx.objectStore(STORE_CACHE).get(key);
      req.onsuccess = () => res(req.result?.data ?? null);
      req.onerror = () => rej(req.error);
    });
  } catch { return null; }
}

export async function cacheSet(key, data) {
  try {
    const db = await openDB();
    const tx = db.transaction(STORE_CACHE, 'readwrite');
    tx.objectStore(STORE_CACHE).put({ key, data, ts: Date.now() });
    return new Promise((res, rej) => { tx.oncomplete = res; tx.onerror = () => rej(tx.error); });
  } catch {}
}

export async function cacheGetAge(key) {
  try {
    const db = await openDB();
    const tx = db.transaction(STORE_CACHE, 'readonly');
    return new Promise((res, rej) => {
      const req = tx.objectStore(STORE_CACHE).get(key);
      req.onsuccess = () => {
        if (!req.result) { res(-1); return; }
        res(Date.now() - (req.result.ts || 0));
      };
      req.onerror = () => rej(req.error);
    });
  } catch { return -1; }
}

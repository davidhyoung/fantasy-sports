/**
 * Per-viewer filter/sort persistence (localStorage) — "remember where I
 * left this page's filters" across a reload, never shared, never read by
 * the server. One key per page (or per page+entity, e.g. league ID), one
 * JSON object holding every filter dimension that page wants to remember.
 *
 * Every access is wrapped in try/catch: a private window, blocked site
 * data, or a full quota can throw, and losing the remembered filters is a
 * minor inconvenience, not worth surfacing an error for.
 */

export function loadPersistedFilters<T extends object>(key: string): Partial<T> {
  try {
    const raw = localStorage.getItem(key)
    return raw ? JSON.parse(raw) : {}
  } catch {
    return {}
  }
}

export function savePersistedFilters<T extends object>(key: string, filters: T) {
  try {
    localStorage.setItem(key, JSON.stringify(filters))
  } catch {
    // Private window, blocked site data, quota — see module doc.
  }
}

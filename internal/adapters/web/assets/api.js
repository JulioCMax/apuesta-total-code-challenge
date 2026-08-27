/**
 * Thin client over the public HTTP contract.
 *
 * This module knows nothing about rendering and holds no business rule: it
 * maps each endpoint to a function and turns the API's single error
 * envelope into a typed exception. Every rule the UI appears to enforce
 * (same-event combos, stake bounds, balance) is decided by the server and
 * surfaced from here.
 */

const BASE = '/api/v1';
const TOKEN_KEY = 'at.token';

/**
 * ApiError carries the whole error envelope, not just a message. The `code`
 * is what callers branch on: it is the API's stable contract, while
 * `message` is human-facing Spanish that may be reworded, and `details`
 * holds the per-code payload (stake bounds, the offending event, the
 * balance shortfall).
 */
export class ApiError extends Error {
  constructor(status, payload) {
    const body = (payload && payload.error) || {};
    super(body.message || 'No se pudo completar la operación.');
    this.name = 'ApiError';
    this.status = status;
    this.code = body.code || 'UNKNOWN';
    this.details = body.details || null;
    this.requestId = (payload && payload.requestId) || '';
  }
}

/**
 * NetworkError separates "the request never reached the API" from "the API
 * answered with an error". Only the first is worth suggesting a retry for.
 */
export class NetworkError extends Error {
  constructor(cause) {
    super('No se pudo contactar con la API. ¿Está levantado el servicio?');
    this.name = 'NetworkError';
    this.code = 'NETWORK_ERROR';
    this.cause = cause;
  }
}

/*
 * Token persistence is best-effort. localStorage throws outright in a
 * private window or when site data is blocked, and a demo client must not
 * fail to start because a browser refused to remember a session.
 */
let token = readStoredToken();

function readStoredToken() {
  try {
    return window.localStorage.getItem(TOKEN_KEY) || '';
  } catch {
    return '';
  }
}

export function getToken() {
  return token;
}

export function setToken(value) {
  token = value || '';
  try {
    if (token) window.localStorage.setItem(TOKEN_KEY, token);
    else window.localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* Session lives in memory for this tab only. */
  }
}

async function request(path, options = {}) {
  const { method = 'GET', body, auth = false, headers = {} } = options;
  const init = { method, headers: { Accept: 'application/json', ...headers } };

  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }
  if (auth && token) {
    init.headers.Authorization = `Bearer ${token}`;
  }

  let response;
  try {
    response = await fetch(BASE + path, init);
  } catch (cause) {
    throw new NetworkError(cause);
  }

  // 204 and empty bodies are legitimate; a body that fails to parse is not
  // treated as fatal on its own, because the status code still classifies
  // the outcome and an unparsed body only costs us the error detail.
  const text = await response.text();
  let payload = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = null;
    }
  }

  if (!response.ok) {
    throw new ApiError(response.status, payload);
  }
  return payload;
}

/** GET /events — optional from/to accept "YYYY-MM-DD" or RFC3339. */
export function listEvents({ from, to } = {}) {
  const params = new URLSearchParams();
  if (from) params.set('from', from);
  if (to) params.set('to', to);
  const query = params.toString();
  return request(`/events${query ? `?${query}` : ''}`);
}

/** GET /events/:id — includes markets already in the API's default order. */
export function eventDetail(id) {
  return request(`/events/${encodeURIComponent(id)}`);
}

/**
 * POST /betslip/calculate — public; prices singles and the combo.
 *
 * isBetBuilder is sent exactly as the caller passes it (default false):
 * it must only ever be true when the caller has explicitly enabled the
 * Bet Builder toggle, never inferred from the selections themselves — the
 * server enforces the actual rule either way.
 */
export function calculate({ selectionIds, stake, isBetBuilder = false }) {
  return request('/betslip/calculate', {
    method: 'POST',
    body: { selectionIds, stake, isBetBuilder },
  });
}

/**
 * POST /betslip/place — authenticated.
 *
 * The Idempotency-Key is generated per placement attempt so a retry of the
 * *same* attempt is deduplicated by the API, while a deliberate second bet
 * carries a new key and is a genuinely new placement. isBetBuilder mirrors
 * calculate()'s own field.
 */
export function place({ selectionIds, stake, idempotencyKey, isBetBuilder = false }) {
  const headers = {};
  if (idempotencyKey) headers['Idempotency-Key'] = idempotencyKey;
  return request('/betslip/place', {
    method: 'POST',
    auth: true,
    headers,
    body: { selectionIds, stake, isBetBuilder },
  });
}

/** POST /auth/login — returns { token, expiresIn }. */
export function login({ email, password }) {
  return request('/auth/login', { method: 'POST', body: { email, password } });
}

/** GET /balance — authenticated. */
export function balance() {
  return request('/balance', { auth: true });
}

/** GET /bets — authenticated bet history for the caller. */
export function bets({ limit } = {}) {
  const query = limit ? `?limit=${encodeURIComponent(limit)}` : '';
  return request(`/bets${query}`, { auth: true });
}

/** Random idempotency key, with a fallback for non-secure contexts. */
export function newIdempotencyKey() {
  if (window.crypto && typeof window.crypto.randomUUID === 'function') {
    return window.crypto.randomUUID();
  }
  return `k-${Date.now()}-${Math.random().toString(16).slice(2, 10)}`;
}

/**
 * Presentation helpers: money, dates and team identity.
 *
 * None of these decide anything. Amounts and odds already arrive from the
 * API as fixed-2 JSON numbers, so formatting never re-rounds a value the
 * server computed — it only renders it.
 */

const DOW = ['dom', 'lun', 'mar', 'mié', 'jue', 'vie', 'sáb'];
const MONTHS = [
  'enero', 'febrero', 'marzo', 'abril', 'mayo', 'junio',
  'julio', 'agosto', 'septiembre', 'octubre', 'noviembre', 'diciembre',
];

/** Currency symbols for the currencies this demo actually serves. */
const SYMBOLS = { PEN: 'S/', USD: '$', EUR: '€' };

export function money(value, currency = 'PEN') {
  const amount = Number(value ?? 0);
  const symbol = SYMBOLS[currency] || `${currency} `;
  return `${symbol} ${amount.toFixed(2)}`;
}

export function odds(value) {
  return Number(value ?? 0).toFixed(2);
}

export function time(iso) {
  const d = new Date(iso);
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

/**
 * Local calendar day of an instant, as "YYYY-MM-DD".
 *
 * Deliberately built from the local getters rather than toISOString(),
 * which would shift the day across the UTC boundary and file a 21:00 local
 * kickoff under tomorrow's heading.
 */
export function dayKey(iso) {
  const d = new Date(iso);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

export function dayOfWeek(key) {
  return DOW[dayFromKey(key).getDay()];
}

export function dayOfMonth(key) {
  return dayFromKey(key).getDate();
}

export function monthName(key) {
  return MONTHS[dayFromKey(key).getMonth()];
}

/** Shifts a "YYYY-MM-DD" key by whole days, returning the same shape. */
export function addDays(key, days) {
  const d = dayFromKey(key);
  d.setDate(d.getDate() + days);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/** Parses "YYYY-MM-DD" as a *local* midnight, never as UTC. */
function dayFromKey(key) {
  const [y, m, d] = String(key).split('-').map(Number);
  return new Date(y, (m || 1) - 1, d || 1);
}

/*
 * Country codes rather than flag emoji.
 *
 * Regional-indicator flag emoji do not render as flags on Windows — the
 * platform font shows the bare two-letter sequence instead. A design that
 * silently degrades to "MX" on the reviewer's own machine is worse than one
 * that shows a deliberate, identical chip everywhere.
 */
const CODES = {
  'méxico': 'MEX',
  'sudáfrica': 'RSA',
  'corea del sur': 'KOR',
  'república de corea': 'KOR',
  'canadá': 'CAN',
  'catar': 'QAT',
  'suiza': 'SUI',
  'brasil': 'BRA',
  'marruecos': 'MAR',
  'haití': 'HAI',
  'escocia': 'SCO',
  'ee.uu.': 'USA',
  'estados unidos': 'USA',
  'paraguay': 'PAR',
  'australia': 'AUS',
  'alemania': 'GER',
  'curazao': 'CUW',
  'costa de marfil': 'CIV',
  'ecuador': 'ECU',
  'países bajos': 'NED',
  'japón': 'JPN',
  'bélgica': 'BEL',
  'egipto': 'EGY',
  'irán': 'IRN',
  'nueva zelanda': 'NZL',
  'españa': 'ESP',
  'cabo verde': 'CPV',
  'uruguay': 'URU',
  'arabia saudita': 'KSA',
  'francia': 'FRA',
  'senegal': 'SEN',
  'argentina': 'ARG',
  'argelia': 'ALG',
  'austria': 'AUT',
  'jordania': 'JOR',
  'colombia': 'COL',
  'uzbekistán': 'UZB',
  'inglaterra': 'ENG',
  'croacia': 'CRO',
  'ghana': 'GHA',
  'panamá': 'PAN',
};

/**
 * Three-letter code for a team, or the first letters of its name when the
 * catalog carries a placeholder such as "Por definir" — the dataset has
 * genuinely unresolved fixtures and inventing a country for them would be
 * fabricating data.
 */
export function teamCode(name) {
  if (!name) return '—';
  const code = CODES[String(name).trim().toLowerCase()];
  if (code) return code;
  return String(name).replace(/[^\p{L}]/gu, '').slice(0, 3).toUpperCase() || '—';
}

/** True when the catalog could not resolve this side of the fixture yet. */
export function isUndefinedRival(name) {
  return !name || /por definir/i.test(name);
}

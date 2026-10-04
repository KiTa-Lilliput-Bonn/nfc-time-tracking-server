const euro = new Intl.NumberFormat('de-DE', { style: 'currency', currency: 'EUR' })

/** Cent → „1.234,56 €“ */
export function formatEuro(cents: number): string {
  return euro.format(cents / 100)
}

/** „12,50“, „12.50“, „1.234,5“ → Cent; null bei ungültiger Eingabe. */
export function parseEuroToCents(input: string): number | null {
  let s = input.trim().replace(/\s|€/g, '')
  if (!s) return null
  if (s.includes(',')) {
    s = s.replace(/\./g, '').replace(',', '.')
  } else if (/^\d{1,3}(\.\d{3})+$/.test(s)) {
    s = s.replace(/\./g, '')
  }
  if (!/^\d+(\.\d{1,2})?$/.test(s)) return null
  return Math.round(parseFloat(s) * 100)
}

/** Cent → Eingabewert „12,50“ */
export function centsToInput(cents: number): string {
  return (cents / 100).toFixed(2).replace('.', ',')
}

const monthNames = [
  'Januar',
  'Februar',
  'März',
  'April',
  'Mai',
  'Juni',
  'Juli',
  'August',
  'September',
  'Oktober',
  'November',
  'Dezember',
]

/** „2026-04“ → „April 2026“ */
export function monthLabel(ym: string): string {
  const [y, m] = ym.split('-').map(Number)
  if (!y || !m) return ym
  return `${monthNames[m - 1]} ${y}`
}

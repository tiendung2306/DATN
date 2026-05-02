/**
 * Counts Unicode code points (scalar values), aligned with Go's utf8.RuneCountInString
 * for typical BMP + supplementary-plane text.
 */
export function countUnicodeRunes(s: string): number {
  return [...s].length
}

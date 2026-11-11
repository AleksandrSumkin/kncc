/**
 * Formats duration in milliseconds to human-readable format.
 * - If ms < 1000: 'X ms'
 * - If ms >= 1000: 'X.Xs' (with 1 decimal digit, e.g. 21.3s)
 * - If ms <= 0 or not provided: ''
 */
export function formatDuration(ms?: number | null): string {
  if (ms === undefined || ms === null || isNaN(ms) || ms <= 0) {
    return '';
  }
  if (ms < 1000) {
    return `${Math.round(ms)} ms`;
  }
  const seconds = ms / 1000;
  return `${seconds.toFixed(1)}s`;
}

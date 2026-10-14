// Short text for a QR payload. A URL shows its host. Anything else is cut on
// a character boundary. Keep this in step with QRPreview in internal/scan/qr.go.

const qrPreviewRunes = 48;

export function qrPreview(raw: string): string {
  const value = raw.trim();
  if (value === '') return '';
  const host = qrHost(value);
  if (host !== null) return shorten(host, qrPreviewRunes);
  return shorten(value, qrPreviewRunes);
}

function qrHost(value: string): string | null {
  const lower = value.toLowerCase();
  let candidate: string;
  if (lower.startsWith('https://') || lower.startsWith('http://')) {
    candidate = value;
  } else if (lower.startsWith('www.')) {
    candidate = `https://${value}`;
  } else {
    return null;
  }
  try {
    const host = new URL(candidate).hostname.toLowerCase();
    return host === '' ? null : host;
  } catch {
    return null;
  }
}

function shorten(value: string, maxRunes: number): string {
  const runes = Array.from(value);
  if (runes.length <= maxRunes) return value;
  if (maxRunes <= 1) return '…';
  return `${runes.slice(0, maxRunes - 1).join('')}…`;
}

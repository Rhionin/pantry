import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  deliver,
  resetSSETracking,
  routeFromPath,
  sanitizeMessage,
  sanitizePage,
  sseGap,
} from './client';

afterEach(() => {
  resetSSETracking();
});

describe('sanitizePage', () => {
  it('drops query strings and fragments that may contain barcodes', () => {
    expect(sanitizePage('/inventory?barcode=123456789012')).toBe('/inventory');
    expect(sanitizePage('/shopping#milk')).toBe('/shopping');
    expect(sanitizePage('https://pantry.local/queue?barcode=111')).toBe('/queue');
  });
});

describe('sanitizeMessage', () => {
  it('removes query text from error messages', () => {
    expect(sanitizeMessage('failed ?barcode=123456789012 now')).toBe('failed now');
  });
});

describe('routeFromPath', () => {
  it('keeps the path and drops the barcode query', () => {
    expect(routeFromPath('/api/products/lookup?barcode=123456789012')).toBe('/api/products/lookup');
  });
});

describe('sseGap', () => {
  it('treats the first id as a baseline and later jumps as missed events', () => {
    expect(sseGap(0, '1')).toEqual({ next: 1, skipped: 0 });
    expect(sseGap(1, '2')).toEqual({ next: 2, skipped: 0 });
    expect(sseGap(2, '5')).toEqual({ next: 5, skipped: 2 });
    expect(sseGap(5, '')).toBeNull();
    expect(sseGap(5, 'nope')).toBeNull();
  });
});

describe('deliver', () => {
  it('posts a report with the query string removed', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    deliver({
      kind: 'js_error',
      page: '/inventory?barcode=123456789012',
      message: 'load failed ?barcode=123456789012',
      session: 'abc',
    }, fetchMock);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/telemetry/client');
    expect(init.method).toBe('POST');
    const body = JSON.parse(String(init.body)) as { page: string; message: string; session: string };
    expect(body.page).toBe('/inventory');
    expect(body.message).not.toContain('123456789012');
    expect(body.session).toBe('abc');
  });

  it('swallows a fetch that rejects', () => {
    const fetchMock = vi.fn().mockRejectedValue(new Error('offline'));
    expect(() => deliver({ kind: 'sse_error', page: '/' }, fetchMock)).not.toThrow();
  });
});

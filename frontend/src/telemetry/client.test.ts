import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  assetTotals,
  deliver,
  firstPaintRoute,
  notePageLoad,
  pageLoadTiming,
  resetPageLoadTracking,
  resetSSETracking,
  routeFromPath,
  sanitizeMessage,
  sanitizePage,
  sseGap,
} from './client';

afterEach(() => {
  resetSSETracking();
  resetPageLoadTracking();
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

describe('pageLoadTiming', () => {
  it('uses the clock when loadEventEnd and duration are still 0', () => {
    expect(pageLoadTiming({
      loadEventEnd: 0,
      duration: 0,
      responseStart: 202,
      domContentLoadedEventEnd: 1297,
      responseEnd: 400,
    }, 1400)).toEqual({
      durationMs: 1400,
      ttfbMs: 202,
      domContentLoadedMs: 1297,
    });
  });

  it('keeps loadEventEnd once the browser has filled it in', () => {
    expect(pageLoadTiming({
      loadEventEnd: 1500,
      duration: 1500,
      responseStart: 202,
      domContentLoadedEventEnd: 1297,
    }, 1600).durationMs).toBe(1500);
  });
});

describe('assetTotals', () => {
  it('counts JS and CSS bytes without keeping names or query strings', () => {
    const totals = assetTotals([
      { name: 'https://pantry.example/assets/app.js?barcode=123456789012', initiatorType: 'script', transferSize: 800, encodedBodySize: 2000 },
      { name: '/assets/index.css', initiatorType: 'link', transferSize: 100, encodedBodySize: 100 },
      { name: '/api/inventory?q=milk', initiatorType: 'fetch', transferSize: 4000, encodedBodySize: 4000 },
    ]);
    expect(totals).toEqual({
      jsResources: 1,
      jsTransferBytes: 800,
      jsEncodedBytes: 2000,
      cssResources: 1,
      cssTransferBytes: 100,
      cssEncodedBytes: 100,
    });
    expect(JSON.stringify(totals)).not.toContain('123456789012');
    expect(JSON.stringify(totals)).not.toContain('milk');
  });
});

describe('firstPaintRoute', () => {
  it('attributes boot calls and ignores lookups, queries, and later polls', () => {
    expect(firstPaintRoute('/api/scans?userId=user-1&status=pending', 100)).toBe(true);
    expect(firstPaintRoute('/api/scans?status=flagged', 120)).toBe(true);
    expect(firstPaintRoute('/api/products/lookup?barcode=123456789012', 130)).toBe(false);
    expect(firstPaintRoute('/api/inventory', 140)).toBe(true);
    notePageLoad(200);
    expect(firstPaintRoute('/api/scanner/config', 200 + 1500)).toBe(true);
    expect(firstPaintRoute('/api/products', 200 + 1501)).toBe(false);
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

  it('posts page load bytes and a first-paint flag without resource names', () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    deliver({
      kind: 'page_load',
      page: '/',
      durationMs: 1400,
      ttfbMs: 202,
      domContentLoadedMs: 1297,
      jsResources: 2,
      jsTransferBytes: 800,
      cssResources: 1,
      cssTransferBytes: 100,
    }, fetchMock);
    deliver({
      kind: 'api',
      route: '/api/inventory',
      status: 200,
      durationMs: 34,
      firstPaint: true,
    }, fetchMock);

    const pageBody = JSON.parse(String(fetchMock.mock.calls[0][1].body)) as Record<string, unknown>;
    expect(pageBody.durationMs).toBe(1400);
    expect(pageBody.jsTransferBytes).toBe(800);
    expect(pageBody.cssResources).toBe(1);
    expect(JSON.stringify(pageBody)).not.toContain('barcode');

    const apiBody = JSON.parse(String(fetchMock.mock.calls[1][1].body)) as { firstPaint?: boolean; route: string };
    expect(apiBody.route).toBe('/api/inventory');
    expect(apiBody.firstPaint).toBe(true);
  });

  it('swallows a fetch that rejects', () => {
    const fetchMock = vi.fn().mockRejectedValue(new Error('offline'));
    expect(() => deliver({ kind: 'sse_error', page: '/' }, fetchMock)).not.toThrow();
  });
});

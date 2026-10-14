import { describe, expect, it } from 'vitest';
import { qrPreview } from './qrPreview';

describe('qrPreview', () => {
  it('shows the host of a URL and leaves a short payload whole', () => {
    expect(qrPreview('https://world.openfoodfacts.org/product/3017620422003')).toBe('world.openfoodfacts.org');
    expect(qrPreview('http://example.com/a')).toBe('example.com');
    expect(qrPreview('www.kroger.com/p/milk')).toBe('www.kroger.com');
    expect(qrPreview('HTTPS://Shop.Example.com/x')).toBe('shop.example.com');
    expect(qrPreview('hello')).toBe('hello');
    expect(qrPreview('  buy milk  ')).toBe('buy milk');
    expect(qrPreview('123456789012345')).toBe('123456789012345');
    expect(qrPreview('')).toBe('');
  });

  it('cuts a long payload on a character boundary', () => {
    const preview = qrPreview('你'.repeat(60));
    expect(Array.from(preview)).toHaveLength(48);
    expect(preview.endsWith('…')).toBe(true);
    expect(preview.includes('你你')).toBe(true);
  });
});

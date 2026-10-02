import { describe, expect, it } from 'vitest';
import { barcodeOutline, outlineAttribute } from './cameraOverlay';

describe('barcodeOutline', () => {
  it('keeps the four corners a detector already reported', () => {
    const corners = [
      { x: 1, y: 2 },
      { x: 9, y: 2 },
      { x: 9, y: 6 },
      { x: 1, y: 6 },
    ];
    expect(barcodeOutline(corners)).toEqual(corners);
    expect(outlineAttribute(corners)).toBe('1,2 9,2 9,6 1,6');
  });

  it('turns the two ends of a 1D code into a band', () => {
    const outline = barcodeOutline([{ x: 0, y: 10 }, { x: 100, y: 10 }]);
    expect(outline).toHaveLength(4);
    const ys = outline.map((point) => point.y);
    expect(Math.min(...ys)).toBeLessThan(10);
    expect(Math.max(...ys)).toBeGreaterThan(10);
  });

  it('returns nothing when the detector saw no location', () => {
    expect(barcodeOutline([])).toEqual([]);
    expect(outlineAttribute([{ x: 1, y: 1 }])).toBe('');
  });
});

import { describe, expect, it } from 'vitest';
import { placeOnHandLabel } from './placeLabel';

const sizes = [14, 13, 12, 11];
// About the long "18.3 oz on hand · ≈ 5 wks" line: ~12px per font pixel.
const long = (fontSize: number) => fontSize * 12;

describe('placeOnHandLabel', () => {
  it('puts a line that fits in the yellow, with padding', () => {
    const placed = placeOnHandLabel({
      wellWidth: 360,
      fillWidth: 330,
      padding: 10,
      sizes,
      textWidth: long,
    });
    expect(placed.region).toBe('fill');
    expect(placed.fontSize).toBe(14);
    expect(placed.left).toBe(10);
    expect(placed.left + long(placed.fontSize)).toBeLessThanOrEqual(330 - 10);
  });

  it('starts a long line just past a low fill', () => {
    const placed = placeOnHandLabel({
      wellWidth: 360,
      fillWidth: 28,
      padding: 10,
      sizes,
      textWidth: long,
    });
    expect(placed.region).toBe('empty');
    expect(placed.fontSize).toBe(14);
    expect(placed.left).toBe(38);
    expect(placed.left).toBeGreaterThanOrEqual(28 + 10);
    expect(placed.left + long(14)).toBeLessThanOrEqual(360 - 10);
  });

  it('shrinks a long line at a mid fill until it fits in the yellow', () => {
    const placed = placeOnHandLabel({
      wellWidth: 360,
      fillWidth: 180,
      padding: 10,
      sizes,
      textWidth: long,
    });
    const line = long(placed.fontSize);
    expect(placed.region).toBe('fill');
    expect(placed.fontSize).toBeLessThan(14);
    expect(placed.left).toBeGreaterThanOrEqual(10);
    expect(placed.left + line).toBeLessThanOrEqual(180 - 10 + 0.5);
  });

  it('uses the empty side when even the smallest line misses the yellow', () => {
    const placed = placeOnHandLabel({
      wellWidth: 315,
      fillWidth: 140,
      padding: 10,
      sizes,
      textWidth: long,
    });
    const line = long(placed.fontSize);
    expect(placed.region).toBe('empty');
    expect(placed.fontSize).toBe(12);
    expect(placed.left).toBeGreaterThanOrEqual(140 + 10 - 0.5);
    expect(placed.left + line).toBeLessThanOrEqual(315 - 10 + 0.5);
  });

  it('keeps a full bar and an empty bar off the boundary', () => {
    const full = placeOnHandLabel({
      wellWidth: 360,
      fillWidth: 360,
      padding: 10,
      sizes,
      textWidth: long,
    });
    expect(full.region).toBe('fill');
    expect(full.left + long(full.fontSize)).toBeLessThanOrEqual(360 - 10);

    const empty = placeOnHandLabel({
      wellWidth: 360,
      fillWidth: 0,
      padding: 10,
      sizes,
      textWidth: (fontSize) => fontSize * 8,
    });
    expect(empty.region).toBe('empty');
    expect(empty.left).toBe(10);
  });
});

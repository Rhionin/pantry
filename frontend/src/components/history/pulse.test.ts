import { describe, expect, it } from 'vitest';
import {
  buildPulseModel, pulseEmptyCopy, pulseHumpPath, pulseHumpY, pulseLegend, pulseStartLabel, pulseSummary,
} from './pulse';

const now = new Date(2026, 9, 14, 16, 0, 0);

function at(month: number, day: number, hour = 12): string {
  return new Date(2026, month, day, hour, 0, 0).toISOString();
}

describe('pulse chart', () => {
  it('keeps the last 30 calendar days, including today, and drops older moves', () => {
    const model = buildPulseModel([
      { at: at(9, 14, 15), direction: 'in', quantity: 2 },
      { at: at(8, 15, 9), direction: 'out', quantity: 1 },
      { at: at(8, 14, 9), direction: 'in', quantity: 9 },
    ], now, 30);

    expect(model.start).toEqual(new Date(2026, 8, 15));
    expect(model.end).toEqual(new Date(2026, 9, 14, 23, 59, 59, 999));
    expect(model.marks.map((mark) => mark.quantity)).toEqual([1, 2]);
    expect(model.stockedIn).toBe(2);
    expect(model.used).toBe(1);
    expect(pulseSummary(model)).toBe('Last 30 days. 2 units stocked in. 1 unit used.');

    const wider = buildPulseModel([
      { at: at(8, 14, 9), direction: 'in', quantity: 9 },
    ], now, 90);
    expect(wider.marks).toHaveLength(1);
    expect(pulseSummary(wider)).toBe('Last 90 days. 9 units stocked in. No units used.');
  });

  it('scales height to the amount, up for stocked in and down for used', () => {
    const model = buildPulseModel([
      { at: at(9, 14, 9), direction: 'in', quantity: 4 },
      { at: at(9, 10, 9), direction: 'out', quantity: 2 },
      { at: at(9, 8, 9), direction: 'in', quantity: 1 },
    ], now, 30);

    expect(model.bucketed).toBe(false);
    expect(model.maxQuantity).toBe(4);
    expect(model.marks.map((mark) => mark.magnitude)).toEqual([0.25, -0.5, 1]);
    expect(pulseLegend(model)).toBe(
      'Stocked in rises above the line. Used falls below it. Each line is one move. Height is the number of units.',
    );
  });

  it('buckets a busy day into one line per direction and adds the amounts', () => {
    const model = buildPulseModel([
      { at: at(9, 14, 8), direction: 'in', quantity: 2 },
      { at: at(9, 14, 15), direction: 'in', quantity: 4 },
      { at: at(9, 14, 18), direction: 'out', quantity: 1 },
      { at: at(9, 12, 11), direction: 'out', quantity: 3 },
    ], now, 7);

    expect(model.bucketed).toBe(true);
    expect(model.start).toEqual(new Date(2026, 9, 8));
    const today = model.marks.filter((mark) => mark.at === new Date(2026, 9, 14, 12).getTime());
    expect(today).toEqual([
      { at: new Date(2026, 9, 14, 12).getTime(), direction: 'in', quantity: 6, magnitude: 1 },
      { at: new Date(2026, 9, 14, 12).getTime(), direction: 'out', quantity: 1, magnitude: -1 / 6 },
    ]);
    expect(model.marks.find((mark) => mark.direction === 'out' && mark.quantity === 3)?.magnitude).toBe(-0.5);
    expect(model.stockedIn).toBe(6);
    expect(model.used).toBe(4);
    expect(pulseLegend(model)).toContain('Each line is one day.');
    expect(pulseSummary(model)).toBe('Last 7 days. 6 units stocked in. 4 units used.');
  });

  it('rounds a pulse into a hump that keeps its height and stays on one side of the line', () => {
    const baseline = 140;
    const above = 28;
    const rising = [-1, -0.75, -0.5, -0.25, 0].map((t) => pulseHumpY(t, above, baseline));
    const falling = [0, 0.25, 0.5, 0.75, 1].map((t) => pulseHumpY(t, above, baseline));
    expect(rising[0]).toBe(baseline);
    expect(rising[4]).toBe(above);
    expect(falling[4]).toBe(baseline);
    for (let i = 0; i < rising.length - 1; i += 1) expect(rising[i]).toBeGreaterThan(rising[i + 1]);
    for (let i = 0; i < falling.length - 1; i += 1) expect(falling[i]).toBeLessThan(falling[i + 1]);
    for (const y of [...rising, ...falling]) {
      expect(y).toBeLessThanOrEqual(baseline);
      expect(y).toBeGreaterThanOrEqual(above);
    }
    // The shoulder sits closer to the peak than a straight line would.
    expect(pulseHumpY(-0.25, above, baseline)).toBeLessThan(baseline + (above - baseline) * 0.75);

    const below = 210;
    const down = [-1, -0.5, 0, 0.5, 1].map((t) => pulseHumpY(t, below, baseline));
    expect(down[2]).toBe(below);
    for (const y of down) {
      expect(y).toBeGreaterThanOrEqual(baseline);
      expect(y).toBeLessThanOrEqual(below);
    }
    expect(down[1]).toBeGreaterThan(down[0]);
    expect(down[1]).toBeLessThan(down[2]);

    const path = pulseHumpPath(100, above, 30, baseline);
    expect(path.curve).toBe('M 70 140 C 80 140 90 28 100 28 C 110 28 120 140 130 140');
    expect(path.fill.endsWith('Z')).toBe(true);
    expect(path.curve).not.toContain(' L ');
  });

  it('names an empty window and a start date in another year', () => {
    const empty = buildPulseModel([], now, 30);
    expect(empty.marks).toEqual([]);
    expect(pulseSummary(empty)).toBe('Last 30 days. No units stocked in. No units used.');
    expect(pulseLegend(empty)).toBe(
      'Stocked in rises above the line. Used falls below it. Height is the number of units.',
    );
    expect(pulseEmptyCopy(30)).toBe('Nothing moved in the last 30 days.');

    const january = new Date(2026, 0, 5, 12);
    const model = buildPulseModel([], january, 30);
    expect(pulseStartLabel(model.start, january)).toContain('2025');
    expect(pulseStartLabel(model.start, now)).not.toContain('2026');
  });
});

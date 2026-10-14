import { describe, expect, it } from 'vitest';
import {
  buildPulseModel, pulseEmptyCopy, pulseLegend, pulseStartLabel, pulseSummary,
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

import { describe, expect, it } from 'vitest';
import {
  buildPulseModel, buildPulseSeries, monotoneSlopes, pulseCount, pulseDateTicks, pulseDayFor, pulseDays,
  pulseEmptyCopy, pulseHumpPath, pulseHumpY, pulseLegend, pulseQuantityAt, pulseStartLabel, pulseSummary,
  pulseTickLabel, pulseYTicks,
} from './pulse';

const now = new Date(2026, 9, 14, 16, 0, 0);

function cubicBezier(p0: number, p1: number, p2: number, p3: number, t: number): number {
  const u = 1 - t;
  return (u * u * u * p0) + (3 * u * u * t * p1) + (3 * u * t * t * p2) + (t * t * t * p3);
}

function at(month: number, day: number, hour = 12): string {
  return new Date(2026, month, day, hour, 0, 0).toISOString();
}

describe('pulse chart', () => {
  it('sums two same-day moves of 2 and 3 into a peak of 5', () => {
    const series = buildPulseSeries([
      { at: new Date(2026, 9, 14, 8, 0, 0).toISOString(), direction: 'in', quantity: 2 },
      { at: new Date(2026, 9, 14, 15, 0, 0).toISOString(), direction: 'in', quantity: 3 },
    ], now, 30);
    expect(series.peakIn).toBe(5);
    expect(series.peakOut).toBe(0);
    expect(series.centersIn).toEqual([
      { at: new Date(2026, 9, 14, 12, 0, 0).getTime(), quantity: 5 },
    ]);
  });

  it('adds neighboring days instead of keeping the taller one', () => {
    const series = buildPulseSeries([
      { at: new Date(2026, 9, 10, 12, 0, 0).toISOString(), direction: 'in', quantity: 4 },
      { at: new Date(2026, 9, 11, 12, 0, 0).toISOString(), direction: 'in', quantity: 4 },
    ], now, 30);
    const first = new Date(2026, 9, 10, 12, 0, 0).getTime();
    const between = first + 12 * 60 * 60 * 1000;
    const atFirst = pulseQuantityAt(series.centersIn, first, series.halfMs);
    const atGap = pulseQuantityAt(series.centersIn, between, series.halfMs);
    expect(atFirst).toBeGreaterThan(4);
    expect(atGap).toBeGreaterThan(4);
    expect(atGap).toBeGreaterThan(atFirst - 4);
  });

  it('keeps a monotone span between its endpoints and off the far side of the baseline', () => {
    const xs = [0, 1, 2];
    const ys = [140, 28, 140];
    const slopes = monotoneSlopes(xs, ys);
    expect(slopes[1]).toBe(0);
    for (let step = 0; step <= 8; step += 1) {
      const t = step / 8;
      const y = cubicBezier(140, 140 + slopes[0] / 3, 28 - slopes[1] / 3, 28, t);
      expect(y).toBeLessThanOrEqual(140);
      expect(y).toBeGreaterThanOrEqual(28);
    }
  });

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
    expect(pulseLegend()).toBe(
      'Stocked in rises above the line. Used falls below it. Overlapping moves add together. Height is the number of units.',
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
    expect(pulseLegend()).toContain('Overlapping moves add together.');
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
    expect(pulseLegend()).toBe(
      'Stocked in rises above the line. Used falls below it. Overlapping moves add together. Height is the number of units.',
    );
    expect(pulseEmptyCopy(30)).toBe('Nothing moved in the last 30 days.');

    const january = new Date(2026, 0, 5, 12);
    const model = buildPulseModel([], january, 30);
    expect(pulseStartLabel(model.start, january)).toContain('2025');
    expect(pulseStartLabel(model.start, now)).not.toContain('2026');
  });

  it('starts at the opening stock in so later moves set the scale', () => {
    const setup = new Date(2026, 9, 1, 15, 0, 0).toISOString();
    const moves = [
      { at: new Date(2026, 8, 20, 10, 0, 0).toISOString(), direction: 'in', quantity: 80 },
      { at: new Date(2026, 9, 1, 10, 0, 0).toISOString(), direction: 'in', quantity: 9 },
      { at: new Date(2026, 9, 1, 16, 0, 0).toISOString(), direction: 'in', quantity: 4 },
      { at: new Date(2026, 9, 10, 12, 0, 0).toISOString(), direction: 'out', quantity: 1 },
    ];
    const series = buildPulseSeries(moves, now, 30, setup);

    expect(series.fromSetup).toBe(true);
    expect(series.peakIn).toBe(4);
    expect(series.stockedIn).toBe(4);
    expect(series.used).toBe(1);
    expect(series.samples[0].at).toBe(new Date(setup).getTime());
    expect(series.centersIn).toEqual([
      { at: new Date(setup).getTime(), quantity: 4 },
    ]);
    const since = pulseStartLabel(series.start, now);
    expect(pulseSummary({ rangeDays: 30, stockedIn: series.stockedIn, used: series.used, since }))
      .toBe(`Since ${since}. 4 units stocked in. 1 unit used.`);
    expect(pulseEmptyCopy(30, true)).toBe('Nothing has moved since the pantry was set up.');

    const days = pulseDays(moves, series.start, series.end, now);
    const openingDay = pulseDayFor(days, new Date(2026, 9, 1, 18, 0, 0).getTime());
    expect(openingDay?.stockedIn).toBe(4);
    expect(openingDay?.used).toBe(0);
  });

  it('keeps the chosen range when setup is already behind it', () => {
    const series = buildPulseSeries([
      { at: new Date(2026, 9, 14, 8, 0, 0).toISOString(), direction: 'in', quantity: 2 },
      { at: new Date(2026, 9, 14, 15, 0, 0).toISOString(), direction: 'in', quantity: 3 },
    ], now, 30, new Date(2026, 7, 1, 12, 0, 0).toISOString());

    expect(series.fromSetup).toBe(false);
    expect(series.peakIn).toBe(5);
    expect(series.samples[0].at).toBe(new Date(2026, 8, 15).getTime());
    expect(pulseSummary({ rangeDays: 30, stockedIn: series.stockedIn, used: series.used }))
      .toBe('Last 30 days. 5 units stocked in. No units used.');
  });

  it('marks units on the scale and names the day under a point', () => {
    const moves = [
      { at: new Date(2026, 9, 14, 8, 0, 0).toISOString(), direction: 'in', quantity: 2 },
      { at: new Date(2026, 9, 14, 15, 0, 0).toISOString(), direction: 'in', quantity: 3 },
      { at: new Date(2026, 9, 13, 18, 0, 0).toISOString(), direction: 'out', quantity: 1 },
    ];
    const series = buildPulseSeries(moves, now, 7);
    expect(pulseYTicks(series.peakIn).map((tick) => pulseTickLabel(tick.value))).toEqual(['0', '5', '5', '2.5', '2.5']);
    const days = pulseDays(moves, series.start, series.end, now);
    const today = pulseDayFor(days, new Date(2026, 9, 14, 20, 0, 0).getTime());
    expect(today).toMatchObject({ stockedIn: 5, used: 0 });
    expect(today?.label).toBe(new Intl.DateTimeFormat(undefined, {
      weekday: 'long', month: 'long', day: 'numeric',
    }).format(new Date(2026, 9, 14)));
    expect(pulseDayFor(days, new Date(2026, 9, 13, 1, 0, 0).getTime())).toMatchObject({ stockedIn: 0, used: 1 });
    const ticks = pulseDateTicks(series.start, series.end, now);
    expect(ticks[0].align).toBe('start');
    expect(ticks[ticks.length - 1]).toMatchObject({ label: 'Today', align: 'end' });
    expect(ticks.length).toBeGreaterThanOrEqual(2);
    expect(pulseCount(5, 'stocked in')).toBe('5 stocked in');
    expect(pulseCount(0, 'used')).toBe('0 used');
  });
});

import { ledgerDayKey } from './format';

export const pulseRanges = [7, 30, 90] as const;

export const pulsePlot = {
  width: 1000,
  height: 280,
  baseline: 140,
  amplitude: 112,
};

export type PulseRange = (typeof pulseRanges)[number];

export interface PulseMove {
  at: string;
  direction: string;
  quantity: number;
}

export interface PulseMark {
  at: number;
  direction: 'in' | 'out';
  quantity: number;
  // Positive rises above the baseline. Negative falls below it.
  // The absolute value is this pulse's quantity divided by the tallest one.
  magnitude: number;
}

export interface PulseModel {
  rangeDays: PulseRange;
  start: Date;
  end: Date;
  bucketed: boolean;
  marks: PulseMark[];
  stockedIn: number;
  used: number;
  maxQuantity: number;
}

// The axis is calendar days, so the window starts at local midnight and
// includes today, rather than a rolling count of hours.
export function buildPulseModel(moves: PulseMove[], now: Date, rangeDays: PulseRange): PulseModel {
  const end = endOfDay(now);
  const start = startOfDay(now);
  start.setDate(start.getDate() - (rangeDays - 1));
  const startMs = start.getTime();
  const endMs = end.getTime();
  const inside = moves.filter((move) => {
    const at = new Date(move.at).getTime();
    return at >= startMs && at <= endMs && (move.direction === 'in' || move.direction === 'out') && move.quantity > 0;
  });
  const bucketed = hasDenseDay(inside);
  const combined = bucketed ? bucketByDay(inside) : inside.map(toMark);
  const stockedIn = combined.filter((mark) => mark.direction === 'in').reduce((sum, mark) => sum + mark.quantity, 0);
  const used = combined.filter((mark) => mark.direction === 'out').reduce((sum, mark) => sum + mark.quantity, 0);
  const maxQuantity = combined.reduce((max, mark) => Math.max(max, mark.quantity), 0);
  const marks = combined
    .map((mark) => ({
      ...mark,
      magnitude: maxQuantity === 0 ? 0 : (mark.direction === 'in' ? 1 : -1) * (mark.quantity / maxQuantity),
    }))
    .sort((left, right) => left.at - right.at);
  return { rangeDays, start, end, bucketed, marks, stockedIn, used, maxQuantity };
}

export function pulseSummary(totals: { rangeDays: number; stockedIn: number; used: number; since?: string }): string {
  const lead = totals.since ? `Since ${totals.since}.` : `Last ${totals.rangeDays} days.`;
  return `${lead} ${unitPhrase(totals.stockedIn, 'stocked in')} ${unitPhrase(totals.used, 'used')}`;
}

export function pulseCount(count: number, what: 'stocked in' | 'used'): string {
  return `${count} ${what}`;
}

export function pulseLegend(): string {
  return 'Stocked in rises above the line. Used falls below it. Overlapping moves add together. Height is the number of units.';
}

export interface PulseCenter {
  at: number;
  quantity: number;
}

export interface PulseSample {
  at: number;
  stockedIn: number;
  used: number;
}

export interface PulseSeries {
  samples: PulseSample[];
  peakIn: number;
  peakOut: number;
  centersIn: PulseCenter[];
  centersOut: PulseCenter[];
  halfMs: number;
  start: Date;
  end: Date;
  fromSetup: boolean;
  stockedIn: number;
  used: number;
}

export interface PulseDay {
  key: string;
  at: number;
  label: string;
  stockedIn: number;
  used: number;
}

export interface PulseDateTick {
  at: number;
  label: string;
  align: 'start' | 'center' | 'end';
}

export interface PulseYTick {
  value: number;
  y: number;
}

// Matches the chart: a pulse is a bit wider than a day, and never wider than
// the window can show without covering it.
export function pulseHalfDays(rangeDays: number): number {
  return Math.min(2.05, rangeDays * 0.16);
}

// Same-day moves share one center so their quantities add before the curve is
// drawn. Centers on different days still add wherever their humps overlap.

// The opening stock-in is the chart's start when it falls inside the chosen
// range. A range that begins after that moment keeps its own start.
export function pulseWindow(now: Date, rangeDays: number, setupAt?: string): { start: Date; end: Date; fromSetup: boolean } {
  const end = endOfDay(now);
  const start = startOfDay(now);
  start.setDate(start.getDate() - (rangeDays - 1));
  if (!setupAt) return { start, end, fromSetup: false };
  const setup = new Date(setupAt);
  const setupMs = setup.getTime();
  if (Number.isNaN(setupMs) || setupMs <= start.getTime() || setupMs > end.getTime()) {
    return { start, end, fromSetup: false };
  }
  return { start: setup, end, fromSetup: true };
}

export function buildPulseSeries(moves: PulseMove[], now: Date, rangeDays: PulseRange, setupAt?: string): PulseSeries {
  const window = pulseWindow(now, rangeDays, setupAt);
  const start = window.start;
  const end = window.end;
  const startMs = start.getTime();
  const endMs = end.getTime();
  const halfMs = pulseHalfDays(rangeDays) * 24 * 60 * 60 * 1000;
  const centersIn = dailyCenters(moves, 'in', startMs, endMs);
  const centersOut = dailyCenters(moves, 'out', startMs, endMs);
  const times = new Set<number>([startMs, endMs]);
  const sampleCount = 120;
  for (let i = 1; i < sampleCount; i += 1) {
    times.add(startMs + ((endMs - startMs) * i) / sampleCount);
  }
  for (const center of [...centersIn, ...centersOut]) times.add(center.at);
  const samples = [...times].sort((left, right) => left - right).map((at) => ({
    at,
    stockedIn: pulseQuantityAt(centersIn, at, halfMs),
    used: pulseQuantityAt(centersOut, at, halfMs),
  }));
  return {
    samples,
    peakIn: samples.reduce((max, sample) => Math.max(max, sample.stockedIn), 0),
    peakOut: samples.reduce((max, sample) => Math.max(max, sample.used), 0),
    centersIn,
    centersOut,
    halfMs,
    start,
    end,
    fromSetup: window.fromSetup,
    stockedIn: centersIn.reduce((sum, center) => sum + center.quantity, 0),
    used: centersOut.reduce((sum, center) => sum + center.quantity, 0),
  };
}

export function pulseDays(moves: PulseMove[], start: Date, end: Date, now: Date): PulseDay[] {
  const days: PulseDay[] = [];
  const cursor = startOfDay(start);
  const last = startOfDay(end).getTime();
  const endMs = end.getTime();
  while (cursor.getTime() <= last) {
    const dayStart = cursor.getTime();
    const dayEnd = endOfDay(cursor).getTime();
    const from = Math.max(dayStart, start.getTime());
    let stockedIn = 0;
    let used = 0;
    for (const move of moves) {
      const at = new Date(move.at).getTime();
      if (at < from || at > dayEnd || at > endMs || move.quantity <= 0) continue;
      if (move.direction === 'in') stockedIn += move.quantity;
      else if (move.direction === 'out') used += move.quantity;
    }
    days.push({
      key: ledgerDayKey(cursor),
      at: dayStart,
      label: pulseDayLabel(cursor, now),
      stockedIn,
      used,
    });
    cursor.setDate(cursor.getDate() + 1);
  }
  return days;
}

export function pulseDayFor(days: PulseDay[], at: number): PulseDay | null {
  if (days.length === 0) return null;
  let chosen = days[0];
  for (const day of days) {
    if (day.at <= at) chosen = day;
  }
  return chosen;
}

export function pulseDayLabel(day: Date, now: Date): string {
  const options: Intl.DateTimeFormatOptions = { weekday: 'long', month: 'long', day: 'numeric' };
  if (day.getFullYear() !== now.getFullYear()) options.year = 'numeric';
  return new Intl.DateTimeFormat(undefined, options).format(day);
}

export function pulseDateTicks(start: Date, end: Date, now: Date): PulseDateTick[] {
  const startMs = start.getTime();
  const endMs = end.getTime();
  const span = Math.max(1, endMs - startMs);
  const ticks: PulseDateTick[] = [{
    at: startMs,
    label: sameDay(start, now) ? 'Today' : pulseStartLabel(start, now),
    align: 'start',
  }];
  if (!sameDay(start, end)) {
    for (let slot = 1; slot <= 2; slot += 1) {
      const at = startMs + (span * slot) / 3;
      const day = startOfDay(new Date(at));
      if (sameDay(day, start) || sameDay(day, end)) continue;
      const ratio = (day.getTime() - startMs) / span;
      if (ratio < 0.18 || ratio > 0.82) continue;
      if (ticks.some((tick) => Math.abs(tick.at - day.getTime()) / span < 0.16)) continue;
      ticks.push({
        at: day.getTime(),
        label: sameDay(day, now) ? 'Today' : pulseStartLabel(day, now),
        align: 'center',
      });
    }
    ticks.push({
      at: endMs,
      label: sameDay(end, now) ? 'Today' : pulseStartLabel(end, now),
      align: 'end',
    });
  }
  return ticks;
}

export function pulseTickLabel(value: number): string {
  if (!Number.isFinite(value)) return '0';
  const rounded = Math.abs(value) >= 10 ? Math.round(Math.abs(value)) : Math.round(Math.abs(value) * 10) / 10;
  return Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(1);
}

export function pulseYTicks(scale: number): PulseYTick[] {
  const { baseline, amplitude } = pulsePlot;
  const ticks: PulseYTick[] = [{ value: 0, y: baseline }];
  if (scale > 0) {
    ticks.push({ value: scale, y: baseline - amplitude }, { value: scale, y: baseline + amplitude });
    if (scale >= 2) {
      ticks.push(
        { value: scale / 2, y: baseline - amplitude / 2 },
        { value: scale / 2, y: baseline + amplitude / 2 },
      );
    }
  }
  return ticks;
}

export function pulseQuantityAt(centers: PulseCenter[], at: number, halfMs: number): number {
  return centers.reduce((sum, center) => sum + center.quantity * pulseHumpY((at - center.at) / halfMs, 1, 0), 0);
}

export interface PlotPoint {
  x: number;
  y: number;
}

// Fritsch–Carlson slopes. Each span stays between its endpoints, so the curve
// cannot cross the baseline or rise past the sample that set the height.
export function monotoneSlopes(xs: number[], ys: number[]): number[] {
  const count = xs.length;
  const slopes = new Array<number>(count).fill(0);
  if (count < 2) return slopes;
  const secants: number[] = [];
  for (let i = 0; i < count - 1; i += 1) {
    const dx = xs[i + 1] - xs[i];
    secants.push(dx === 0 ? 0 : (ys[i + 1] - ys[i]) / dx);
  }
  slopes[0] = secants[0];
  slopes[count - 1] = secants[count - 2];
  for (let i = 1; i < count - 1; i += 1) {
    slopes[i] = secants[i - 1] * secants[i] <= 0 ? 0 : (secants[i - 1] + secants[i]) / 2;
  }
  for (let i = 0; i < count - 1; i += 1) {
    if (secants[i] === 0) {
      slopes[i] = 0;
      slopes[i + 1] = 0;
      continue;
    }
    const alpha = slopes[i] / secants[i];
    const beta = slopes[i + 1] / secants[i];
    const sum = alpha * alpha + beta * beta;
    if (sum > 9) {
      const scale = 3 / Math.sqrt(sum);
      slopes[i] = scale * alpha * secants[i];
      slopes[i + 1] = scale * beta * secants[i];
    }
  }
  return slopes;
}

export function monotoneCurve(points: PlotPoint[]): string {
  if (points.length === 0) return '';
  const xs = points.map((point) => point.x);
  const ys = points.map((point) => point.y);
  const slopes = monotoneSlopes(xs, ys);
  let path = `M ${coord(xs[0])} ${coord(ys[0])}`;
  for (let i = 0; i < points.length - 1; i += 1) {
    const dx = xs[i + 1] - xs[i];
    if (dx === 0) continue;
    const c1x = xs[i] + dx / 3;
    const c1y = ys[i] + slopes[i] * dx / 3;
    const c2x = xs[i + 1] - dx / 3;
    const c2y = ys[i + 1] - slopes[i + 1] * dx / 3;
    path += ` C ${coord(c1x)} ${coord(c1y)} ${coord(c2x)} ${coord(c2y)} ${coord(xs[i + 1])} ${coord(ys[i + 1])}`;
  }
  return path;
}

// One filled path for the whole side, closed along the baseline, and one
// stroke that leaves the baseline only while the sum is off zero. The axis
// is drawn separately, so the stroke does not paint it a second time.
export function pulseSidePaths(points: PlotPoint[], baselineY: number): { fill: string; stroke: string } | null {
  if (points.length < 2 || points.every((point) => point.y === baselineY)) return null;
  const curve = monotoneCurve(points);
  const first = points[0];
  const last = points[points.length - 1];
  const fill = `${curve} L ${coord(last.x)} ${coord(baselineY)} L ${coord(first.x)} ${coord(baselineY)} Z`;
  const pieces: string[] = [];
  let run: PlotPoint[] = [];
  const flush = () => {
    if (run.length < 2) {
      run = [];
      return;
    }
    pieces.push(monotoneCurve(run));
    run = [];
  };
  for (let i = 0; i < points.length; i += 1) {
    const off = points[i].y !== baselineY;
    if (off) {
      if (run.length === 0 && i > 0) run.push(points[i - 1]);
      run.push(points[i]);
    } else if (run.length > 0) {
      run.push(points[i]);
      flush();
    }
  }
  flush();
  return { fill, stroke: pieces.join(' ') };
}

function coord(value: number): string {
  const rounded = Math.round(value * 100) / 100;
  return String(rounded);
}

function dailyCenters(moves: PulseMove[], direction: 'in' | 'out', startMs: number, endMs: number): PulseCenter[] {
  const days = new Map<string, PulseCenter>();
  for (const move of moves) {
    if (move.direction !== direction || move.quantity <= 0) continue;
    const when = new Date(move.at);
    const at = when.getTime();
    if (at < startMs || at > endMs) continue;
    const key = ledgerDayKey(when);
    const center = Math.min(endMs, Math.max(startMs, midday(when)));
    const existing = days.get(key);
    if (existing) existing.quantity += move.quantity;
    else days.set(key, { at: center, quantity: move.quantity });
  }
  return [...days.values()];
}

export function pulseEmptyCopy(rangeDays: number, fromSetup = false): string {
  if (fromSetup) return 'Nothing has moved since the pantry was set up.';
  return `Nothing moved in the last ${rangeDays} days.`;
}

// A cubic smoothstep hump. Each half is monotone, the tangent is flat at the
// baseline and at the peak, and the height stays between those two values.
export function pulseHumpY(t: number, peakY: number, baselineY: number): number {
  const clamped = Math.min(1, Math.max(-1, t));
  const towardPeak = 1 - Math.abs(clamped);
  const smooth = towardPeak * towardPeak * (3 - 2 * towardPeak);
  return baselineY + (peakY - baselineY) * smooth;
}

export function pulseHumpPath(x: number, peakY: number, half: number, baselineY: number): { fill: string; curve: string } {
  const left = x - half;
  const right = x + half;
  const curve = [
    `M ${left} ${baselineY}`,
    `C ${x - (half * 2) / 3} ${baselineY} ${x - half / 3} ${peakY} ${x} ${peakY}`,
    `C ${x + half / 3} ${peakY} ${x + (half * 2) / 3} ${baselineY} ${right} ${baselineY}`,
  ].join(' ');
  return { fill: `${curve} Z`, curve };
}

export function pulseStartLabel(start: Date, now: Date): string {
  const options: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' };
  if (start.getFullYear() !== now.getFullYear()) options.year = 'numeric';
  return new Intl.DateTimeFormat(undefined, options).format(start);
}

function unitPhrase(count: number, what: string): string {
  if (count === 0) return `No units ${what}.`;
  if (count === 1) return `1 unit ${what}.`;
  return `${count} units ${what}.`;
}

function toMark(move: PulseMove): PulseMark {
  return {
    at: new Date(move.at).getTime(),
    direction: move.direction === 'in' ? 'in' : 'out',
    quantity: move.quantity,
    magnitude: 0,
  };
}

function hasDenseDay(moves: PulseMove[]): boolean {
  const counts = new Map<string, number>();
  for (const move of moves) {
    const key = ledgerDayKey(new Date(move.at));
    const count = (counts.get(key) ?? 0) + 1;
    if (count > 1) return true;
    counts.set(key, count);
  }
  return false;
}

function bucketByDay(moves: PulseMove[]): PulseMark[] {
  const days = new Map<string, { at: number; inn: number; out: number }>();
  for (const move of moves) {
    const when = new Date(move.at);
    const key = ledgerDayKey(when);
    const day = days.get(key) ?? { at: midday(when), inn: 0, out: 0 };
    if (move.direction === 'in') day.inn += move.quantity;
    else day.out += move.quantity;
    days.set(key, day);
  }
  const marks: PulseMark[] = [];
  for (const day of days.values()) {
    if (day.inn > 0) marks.push({ at: day.at, direction: 'in', quantity: day.inn, magnitude: 0 });
    if (day.out > 0) marks.push({ at: day.at, direction: 'out', quantity: day.out, magnitude: 0 });
  }
  return marks;
}

function sameDay(left: Date, right: Date): boolean {
  return left.getFullYear() === right.getFullYear()
    && left.getMonth() === right.getMonth()
    && left.getDate() === right.getDate();
}

function startOfDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}

function endOfDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate(), 23, 59, 59, 999);
}

function midday(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate(), 12, 0, 0, 0).getTime();
}

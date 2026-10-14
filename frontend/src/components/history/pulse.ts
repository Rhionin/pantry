export const pulseRanges = [7, 30, 90] as const;

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

export function pulseSummary(model: PulseModel): string {
  return `Last ${model.rangeDays} days. ${unitPhrase(model.stockedIn, 'stocked in')} ${unitPhrase(model.used, 'used')}`;
}

export function pulseLegend(model: PulseModel): string {
  const each = model.marks.length === 0
    ? ''
    : model.bucketed
      ? ' Each line is one day.'
      : ' Each line is one move.';
  return `Stocked in rises above the line. Used falls below it.${each} Height is the number of units.`;
}

export function pulseEmptyCopy(rangeDays: number): string {
  return `Nothing moved in the last ${rangeDays} days.`;
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
    const key = dayKey(new Date(move.at));
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
    const key = dayKey(when);
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

function dayKey(date: Date): string {
  return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
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

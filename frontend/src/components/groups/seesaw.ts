import type { NetDimension, ProductGroup } from '../../types';
import { formatAmount, onHandOunces, type MemberPackage } from './copy';

export type SeesawPin = 'amount' | 'time';

export interface SeesawDraft {
  pin: SeesawPin;
  value: number | string;
}

export interface SeesawEnd {
  pin: SeesawPin;
  pinned: boolean;
  label: string;
  known: boolean;
  /** Household window, shown beside the pinned time. */
  isDefault: boolean;
  labelText: string;
}

export interface SeesawView {
  caption: string;
  rateLabel: string;
  unknownHint: string;
  amount: SeesawEnd;
  time: SeesawEnd;
  amountValue: number | null;
  monthsValue: number | null;
  /** Ounces the bin fills against. Absent when the target is not an amount. */
  fillQuantity?: number;
  dimension: NetDimension;
  hasRate: boolean;
}

export function clampMonth(months: number): number {
  return Math.min(12, Math.max(1, Math.round(months)));
}

// Weeks while the span is short, then months. A 30-day month keeps this
// in step with the supply window.
export function formatDuration(months: number): string {
  if (!Number.isFinite(months) || months <= 0) return '';
  const weeks = months * (30 / 7);
  if (weeks < 10) {
    const rounded = Math.round(weeks);
    if (rounded < 1) return '< 1 wk';
    return rounded === 1 ? '1 wk' : `${rounded} wks`;
  }
  const rounded = Math.round(months);
  if (rounded < 1) return '< 1 mo';
  return `${rounded} mo`;
}

const positive = (value: number | null | undefined): value is number =>
  value !== null && value !== undefined && Number.isFinite(value) && value > 0;

function rateOf(group: Pick<ProductGroup, 'usage'>): { perMonth: number; unit: string } | null {
  const usage = group.usage;
  if (!usage || !(usage.perMonth > 0)) return null;
  return usage;
}

function parseDraft(value: number | string): number | null {
  const parsed = typeof value === 'number' ? value : Number(value);
  return positive(parsed) ? parsed : null;
}

export function seesawView(
  group: Pick<ProductGroup, 'quantity' | 'dimension' | 'windowMonths' | 'usage'>,
  members: MemberPackage[],
  householdMonths: number | undefined,
  draft: SeesawDraft | null,
): SeesawView {
  const dimension: NetDimension = group.dimension === 'volume' ? 'volume' : 'mass';
  const rate = rateOf(group);
  const unit = rate?.unit ?? (dimension === 'volume' ? 'fl oz' : 'oz');

  let pin: SeesawPin;
  let isDefault = false;
  let amount: number | null = null;
  let months: number | null = null;

  if (draft) {
    pin = draft.pin;
    if (draft.pin === 'amount') {
      amount = parseDraft(draft.value);
      months = rate && amount !== null ? amount / rate.perMonth : null;
    } else {
      months = parseDraft(draft.value);
      amount = rate && months !== null ? months * rate.perMonth : null;
    }
  } else if (group.quantity !== undefined) {
    pin = 'amount';
    amount = group.quantity;
    if (rate && positive(amount)) months = amount / rate.perMonth;
  } else if (group.windowMonths !== undefined) {
    pin = 'time';
    months = group.windowMonths;
    if (rate && positive(months)) amount = months * rate.perMonth;
  } else {
    pin = 'time';
    isDefault = true;
    months = positive(householdMonths) ? householdMonths : null;
    if (rate && positive(months)) amount = months * rate.perMonth;
  }

  const amountEnd = endAmount(pin, amount, unit);
  const timeEnd = endTime(pin, months, isDefault);
  const onHand = onHandOunces(members, dimension);
  let caption = '';
  if (onHand !== null) {
    caption = `${formatAmount(onHand)} ${unit} on hand`;
    if (onHand > 0 && rate) caption += ` · ≈ ${formatDuration(onHand / rate.perMonth)}`;
  }

  const fillQuantity = positive(amount) && (pin === 'amount' || rate !== null) ? amount : undefined;

  return {
    caption,
    rateLabel: rate ? `at ${formatAmount(rate.perMonth)} ${unit} / mo` : '',
    unknownHint: rate || (amountEnd.known && timeEnd.known) ? '' : 'Fills in once usage is known',
    amount: amountEnd,
    time: timeEnd,
    amountValue: amount,
    monthsValue: months,
    fillQuantity,
    dimension,
    hasRate: rate !== null,
  };
}

function endAmount(pin: SeesawPin, amount: number | null, unit: string): SeesawEnd {
  const pinned = pin === 'amount';
  if (!positive(amount)) {
    return { pin: 'amount', pinned, label: '—', known: false, isDefault: false, labelText: 'Amount unknown, fills in once usage is known' };
  }
  const shown = `${formatAmount(amount)} ${unit}`;
  if (pinned) {
    return { pin: 'amount', pinned: true, label: shown, known: true, isDefault: false, labelText: `${shown}, pinned` };
  }
  return { pin: 'amount', pinned: false, label: `≈ ${shown}`, known: true, isDefault: false, labelText: `About ${shown}` };
}

function endTime(pin: SeesawPin, months: number | null, isDefault: boolean): SeesawEnd {
  const pinned = pin === 'time';
  if (pinned && isDefault && !positive(months)) {
    return {
      pin: 'time',
      pinned: true,
      label: 'Default',
      known: true,
      isDefault: false,
      labelText: 'Household default, pinned',
    };
  }
  if (!positive(months)) {
    return { pin: 'time', pinned, label: '—', known: false, isDefault: false, labelText: 'Time unknown, fills in once usage is known' };
  }
  if (pinned) {
    const shown = `${Math.round(months)} mo`;
    const marker = isDefault ? ', household default' : '';
    return {
      pin: 'time',
      pinned: true,
      label: shown,
      known: true,
      isDefault,
      labelText: `${shown}${marker}, pinned`,
    };
  }
  const shown = formatDuration(months);
  return { pin: 'time', pinned: false, label: `≈ ${shown}`, known: true, isDefault: false, labelText: `About ${shown}` };
}

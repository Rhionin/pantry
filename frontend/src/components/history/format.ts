export function formatPaceNumber(days: number): string {
  const rounded = Math.round(days * 10) / 10;
  return Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(1);
}

export function onHandLabel(count: number): string {
  if (count === 0) return 'None on hand';
  if (count === 1) return '1 on hand';
  return `${count} on hand`;
}

export function daysLeftLabel(days: number): string {
  if (days === 1) return 'about 1 day left';
  return `about ${days} days left`;
}

export function trendLabel(trend: string | undefined): string {
  if (trend === 'steady') return 'Steady compared with the previous 30 days';
  if (trend === 'faster') return 'Faster than the previous 30 days';
  if (trend === 'slower') return 'Slower than the previous 30 days';
  return '';
}

export function moveTitle(direction: string, quantity: number): string {
  if (direction === 'in') return `Stocked in ${quantity}`;
  return `Used ${quantity}`;
}

export function sourceLabel(source: string): string {
  return source === 'scan' ? 'scan' : 'by hand';
}

function sameDay(left: Date, right: Date): boolean {
  return left.getFullYear() === right.getFullYear()
    && left.getMonth() === right.getMonth()
    && left.getDate() === right.getDate();
}

// formatMoveWhen names the calendar day when it is today or yesterday, and
// includes the weekday and date otherwise so "Wed" is not an earlier week.
export function formatMoveWhen(at: Date, now: Date, source: string): string {
  const time = new Intl.DateTimeFormat(undefined, { hour: 'numeric', minute: '2-digit' }).format(at);
  const how = sourceLabel(source);
  if (sameDay(at, now)) return `Today, ${time}, ${how}`;
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (sameDay(at, yesterday)) return `Yesterday, ${time}, ${how}`;
  const date = new Intl.DateTimeFormat(undefined, {
    weekday: 'short', month: 'short', day: 'numeric',
  }).format(at);
  return `${date}, ${time}, ${how}`;
}

export function deltaText(direction: string, quantity: number): string {
  return direction === 'in' ? `+${quantity}` : `−${quantity}`;
}

export function deltaName(direction: string, quantity: number): string {
  if (direction === 'in') {
    return quantity === 1 ? '1 stocked in' : `${quantity} stocked in`;
  }
  return quantity === 1 ? '1 used' : `${quantity} used`;
}

export function undoCopy(direction: string, quantity: number): { title: string; body: string; confirm: string } {
  if (direction === 'in') {
    const shelf = quantity === 1 ? '1 unit leaves the shelf' : `${quantity} units leave the shelf`;
    return {
      title: 'Undo this stock in?',
      body: `${shelf}. The pace is recalculated from the remaining history.`,
      confirm: 'Undo stock in',
    };
  }
  const shelf = quantity === 1 ? '1 unit goes back on the shelf' : `${quantity} units go back on the shelf`;
  return {
    title: 'Undo this use?',
    body: `${shelf}. The pace is recalculated from the remaining history.`,
    confirm: 'Undo this use',
  };
}

export function quantityPrompt(direction: string): string {
  return direction === 'in'
    ? 'How many units should this stock in be?'
    : 'How many units were actually used?';
}

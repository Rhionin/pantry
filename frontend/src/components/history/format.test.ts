import { describe, expect, it } from 'vitest';
import {
  daysLeftLabel, deltaName, deltaText, formatMoveWhen, formatPaceNumber, moveTitle, onHandLabel, quantityPrompt, trendLabel, undoCopy,
} from './format';

describe('history wording', () => {
  const now = new Date(2026, 9, 10, 15, 4, 0);

  it('rounds a pace to one decimal and keeps a whole number whole', () => {
    expect(formatPaceNumber(2.74)).toBe('2.7');
    expect(formatPaceNumber(3)).toBe('3');
  });

  it('names on hand, days left, and the comparison without shorthand', () => {
    expect(onHandLabel(0)).toBe('None on hand');
    expect(onHandLabel(1)).toBe('1 on hand');
    expect(onHandLabel(4)).toBe('4 on hand');
    expect(daysLeftLabel(1)).toBe('about 1 day left');
    expect(daysLeftLabel(11)).toBe('about 11 days left');
    expect(trendLabel('steady')).toBe('Steady compared with the previous 30 days');
    expect(trendLabel('faster')).toBe('Faster than the previous 30 days');
    expect(trendLabel('slower')).toBe('Slower than the previous 30 days');
    expect(trendLabel('')).toBe('');
  });

  it('dates a move so a weekday is not ambiguous', () => {
    expect(formatMoveWhen(new Date(2026, 9, 10, 8, 14, 0), now, 'scan')).toMatch(/^Today, .+scan$/);
    expect(formatMoveWhen(new Date(2026, 9, 9, 18, 42, 0), now, 'scan')).toMatch(/^Yesterday, .+scan$/);
    const older = formatMoveWhen(new Date(2026, 9, 8, 19, 20, 0), now, 'manual');
    expect(older).toContain('Oct');
    expect(older).toContain('8');
    expect(older).toContain('by hand');
  });

  it('labels the quantity change and the undo consequence', () => {
    expect(moveTitle('out', 2)).toBe('Used 2');
    expect(moveTitle('in', 6)).toBe('Stocked in 6');
    expect(deltaText('out', 2)).toBe('−2');
    expect(deltaName('in', 6)).toBe('6 stocked in');
    expect(quantityPrompt('out')).toBe('How many units were actually used?');
    expect(undoCopy('out', 2)).toEqual({
      title: 'Undo this use?',
      body: '2 units go back on the shelf. The pace is recalculated from the remaining history.',
      confirm: 'Undo this use',
    });
    expect(undoCopy('in', 1).body).toBe('1 unit leaves the shelf. The pace is recalculated from the remaining history.');
  });
});

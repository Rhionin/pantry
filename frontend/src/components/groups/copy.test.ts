import { describe, expect, it } from 'vitest';
import { conflictLine, targetChoices } from './copy';

describe('targetChoices', () => {
  it('keeps the group window and one choice per distinct member setting', () => {
    const choices = targetChoices(
      { windowMonths: 6 },
      [
        { productId: 'glacier', name: 'Gatorade Glacier Freeze', quantity: 12, dimension: 'mass' },
        { productId: 'lemon', name: 'Lemonade powder', windowMonths: 3 },
        { productId: 'other', name: 'Other powder', quantity: 12, dimension: 'mass' },
      ],
    );

    expect(choices.map((choice) => choice.label)).toEqual([
      "Keep the group's 6 months",
      'Use 12 ounces (from Gatorade Glacier Freeze, Other powder)',
      'Use 3 months (from Lemonade powder)',
    ]);
    expect(choices[0]?.target).toEqual({ windowMonths: 6 });
    expect(choices[1]?.target).toEqual({ quantity: 12, dimension: 'mass' });
    expect(choices[2]?.target).toEqual({ windowMonths: 3 });
    expect(conflictLine({ productId: 'glacier', name: 'Gatorade Glacier Freeze', quantity: 12, dimension: 'mass' }))
      .toBe('Gatorade Glacier Freeze: 12 ounces');
    expect(conflictLine({ productId: 'lemon', name: 'Lemonade powder', windowMonths: 3 }))
      .toBe('Lemonade powder: 3 months');
  });

  it('uses the household default only when the group has no target', () => {
    const choices = targetChoices(null, [
      { productId: 'glacier', name: 'Gatorade Glacier Freeze', quantity: 12, dimension: 'volume' },
    ]);

    expect(choices[0]).toEqual({ label: 'Keep the household default', target: { clear: true } });
    expect(choices[1]).toEqual({
      label: 'Use 12 fluid ounces (from Gatorade Glacier Freeze)',
      target: { quantity: 12, dimension: 'volume' },
    });
  });

  it('sends the group quantity and its dimension', () => {
    const choices = targetChoices({ quantity: 24, dimension: 'volume' }, []);
    expect(choices).toEqual([
      { label: "Keep the group's 24 fl oz", target: { quantity: 24, dimension: 'volume' } },
    ]);
  });
});

import { describe, expect, it } from 'vitest';
import { binView, conflictLine, detailStockSentence, listSummary, memberLine, ruleSentence, targetChoices } from './copy';

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

const lemon = {
  productId: 'lemon',
  name: 'Gatorade Lemon-Lime',
  onHand: 1,
  unitOfMeasure: 'canister',
  netAmount: 18.3,
  netUnit: 'oz',
};
const glacier = {
  productId: 'glacier',
  name: 'Gatorade Glacier Freeze',
  onHand: 0,
  unitOfMeasure: 'canister',
  netAmount: 50.9,
  netUnit: 'oz',
};

describe('bin sentences', () => {
  it('fills an ounce bin and says what is in it', () => {
    const group = { quantity: 48, dimension: 'mass' as const, rule: 'same_as_ran_out', ruleConfirmed: true, members: [] };
    const view = binView(group, [lemon, glacier]);
    expect(view.corner).toBe('48 oz');
    expect(view.level).toBe('18.3 oz');
    expect(view.segments).toEqual([{ productId: 'lemon', fraction: 18.3 / 48 }]);
    expect(detailStockSentence(group, [lemon, glacier])).toBe(
      '18.3 ounces of Gatorade Lemon-Lime are in the bin. Keep 48 ounces on hand.',
    );
    expect(ruleSentence(group, [lemon], 'detail')).toBe('When this runs out, buy the same kind that ran out.');
    expect(memberLine(glacier)).toBe('50.9 oz canister, none on hand');
  });

  it('leaves the bin empty when the target is months or the household default', () => {
    const months = binView({ windowMonths: 3 }, [{ ...lemon, onHand: 2, unitOfMeasure: 'jar' }]);
    expect(months.segments).toEqual([]);
    expect(months.corner).toBe('3 months');
    expect(detailStockSentence({ windowMonths: 3 }, [{ ...lemon, onHand: 2, unitOfMeasure: 'jar' }])).toBe(
      '2 jars on hand. Keep 3 months.',
    );

    const household = binView({}, [{ productId: 'oat', name: 'Oat milk', onHand: 1, unitOfMeasure: 'carton' }]);
    expect(household.segments).toEqual([]);
    expect(household.corner).toBe('Default');
    expect(listSummary(
      {
        id: 'oat',
        name: 'Oat milk',
        rule: 'same_as_ran_out',
        ruleConfirmed: false,
        members: [],
        runningLow: false,
      },
      [{ productId: 'oat', name: 'Oat milk', onHand: 1, unitOfMeasure: 'carton' }],
      3,
    )).toBe('1 carton on hand. Household default, 3 months. Still using Same as what ran out until you pick a rule.');
  });

  it('names the pinned product in the rule sentence', () => {
    const members = [{ productId: 'creamy', name: 'Creamy peanut butter' }];
    expect(ruleSentence({ rule: 'favorite', ruleConfirmed: true, pinnedProductId: 'creamy' }, members, 'detail'))
      .toBe('When this runs out, always buy Creamy peanut butter.');
    expect(ruleSentence({ rule: 'best_deal', ruleConfirmed: true, pinnedProductId: 'creamy' }, members, 'brief'))
      .toBe('Buy the best deal, or Creamy peanut butter if nothing is on sale.');
  });

  it('does not fill the bin when a stocked package has no size', () => {
    const members = [{ productId: 'lemon', name: 'Gatorade Lemon-Lime', onHand: 1, unitOfMeasure: 'canister' }];
    const view = binView({ quantity: 48, dimension: 'mass' }, members);
    expect(view.segments).toEqual([]);
    expect(detailStockSentence({ quantity: 48, dimension: 'mass' }, members)).toBe(
      '1 canister on hand. Keep 48 ounces on hand. Package sizes are not listed, so the bin stays empty.',
    );
  });
});

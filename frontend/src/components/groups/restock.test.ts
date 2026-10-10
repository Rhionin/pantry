import { describe, expect, it } from 'vitest';
import { everyNoRestock, pickRestock, restockNote, varietyLine } from './restock';

const fancy = {
  productId: 'fancy',
  name: 'Fancy',
  noRestock: true,
  lastConsumedAt: '2026-04-01T00:00:00Z',
  lastStockedAt: '2024-01-01T00:00:00Z',
};
const plain = {
  productId: 'plain',
  name: 'Plain',
  lastConsumedAt: '2026-01-01T00:00:00Z',
  lastStockedAt: '2026-06-01T00:00:00Z',
};

describe('pickRestock', () => {
  it('rotates to the product bought least recently, never-bought first', () => {
    const members = [
      { productId: 'late', name: 'Blueberry', lastStockedAt: '2026-06-01T00:00:00Z' },
      { productId: 'early', name: 'Bran', lastStockedAt: '2024-01-01T00:00:00Z' },
      { productId: 'never', name: 'Corn' },
    ];
    expect(varietyLine(members)).toBe('Next up: Corn · rotates through 3');
    expect(pickRestock('favor_variety', members.slice(0, 2))).toEqual({
      productId: 'early',
      because: 'Next up: Bran · rotates through 2',
    });
  });

  it('breaks a variety tie by name, then id', () => {
    expect(pickRestock('favor_variety', [
      { productId: 'b', name: 'Zucchini' },
      { productId: 'a', name: 'Apple' },
    ]).productId).toBe('a');
    const when = '2026-01-01T00:00:00Z';
    expect(pickRestock('favor_variety', [
      { productId: 'b', name: 'Same', lastStockedAt: when },
      { productId: 'a', name: 'Same', lastStockedAt: when },
    ])).toEqual({ productId: 'a', because: 'Next up: Same · rotates through 2' });
  });

  it('skips don\'t-restock products for every rule', () => {
    const members = [fancy, plain];
    expect(pickRestock('same_as_ran_out', members)).toEqual({
      productId: 'plain',
      because: "Fancy isn't restocked. The last one used up was Plain.",
    });
    expect(pickRestock('favorite', members, 'fancy')).toEqual({
      productId: 'plain',
      because: "Fancy isn't restocked. The last one used up was Plain.",
    });
    expect(pickRestock('best_deal', members, 'fancy')).toEqual({
      productId: 'plain',
      because: "Fancy isn't restocked. Nothing is on sale and no fallback is set. The last one used up was Plain.",
    });
    expect(pickRestock('best_deal', members, 'fancy', [
      { productId: 'fancy', priceCents: 30 },
      { productId: 'plain', priceCents: 80 },
    ]).productId).toBe('plain');
    expect(pickRestock('favor_variety', members)).toEqual({
      productId: 'plain',
      because: 'Next up: Plain · rotates through 1',
    });
  });

  it('buys nothing when every product is marked don\'t restock', () => {
    const members = [
      { productId: 'a', name: 'A', noRestock: true, lastStockedAt: '2026-01-01T00:00:00Z' },
      { productId: 'b', name: 'B', noRestock: true },
    ];
    for (const rule of ['same_as_ran_out', 'favorite', 'best_deal', 'favor_variety']) {
      expect(pickRestock(rule, members, 'a')).toEqual({ productId: '', because: everyNoRestock });
    }
  });
});

describe('restockNote', () => {
  it('names the next product under favor variety and the fallback under a skipped favorite', () => {
    const members = [fancy, plain, { productId: 'corn', name: 'Corn' }];
    expect(restockNote({ rule: 'favor_variety', ruleConfirmed: true }, members))
      .toBe('Next up: Corn · rotates through 2');
    expect(restockNote({ rule: 'favorite', ruleConfirmed: true, pinnedProductId: 'fancy' }, members))
      .toBe("Fancy isn't restocked. The last one used up was Plain.");
    expect(restockNote({ rule: 'favorite', ruleConfirmed: true, pinnedProductId: 'plain' }, members)).toBe('');
    expect(restockNote({ rule: 'favor_variety', ruleConfirmed: false }, members)).toBe('');
  });
});

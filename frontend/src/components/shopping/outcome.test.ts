import fc from 'fast-check';
import { describe, expect, it } from 'vitest';
import { FAILED_NAME_CAP, summarizeNames } from './outcome';

describe('summarizeNames', () => {
  it('keeps a short list intact', () => {
    expect(summarizeNames(['Milk', 'Eggs'])).toEqual({ shown: ['Milk', 'Eggs'], overflow: 0 });
  });

  it('caps the shown names at 50 and counts the rest', () => {
    const names = Array.from({ length: 53 }, (_, index) => `Item ${index}`);
    const summary = summarizeNames(names);
    expect(summary.shown).toHaveLength(FAILED_NAME_CAP);
    expect(summary.overflow).toBe(3);
    expect(summary.shown[0]).toBe('Item 0');
  });

  it('never shows more than the cap and accounts for every name', () => {
    fc.assert(fc.property(
      fc.array(fc.string(), { maxLength: 80 }),
      (names) => {
        const summary = summarizeNames(names);
        expect(summary.shown.length).toBeLessThanOrEqual(FAILED_NAME_CAP);
        expect(summary.shown.length + summary.overflow).toBe(names.length);
        expect(summary.shown).toEqual(names.slice(0, summary.shown.length));
      },
    ));
  });
});

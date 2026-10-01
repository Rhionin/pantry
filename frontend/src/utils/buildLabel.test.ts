import { describe, expect, it } from 'vitest';
import fc from 'fast-check';
import { formatBuildLabel } from './buildLabel';

describe('formatBuildLabel', () => {
  it('preserves the full commit in the visible and accessible label', () => {
    fc.assert(
      fc.property(fc.string(), (commit) => {
        const label = formatBuildLabel(commit);
        if (commit === '') {
          expect(label).toBeNull();
          return;
        }
        expect(label).toEqual({
          text: `build ${commit}`,
          accessibleName: `build ${commit}`,
        });
        expect(label?.text.endsWith(commit)).toBe(true);
        expect(label?.accessibleName.endsWith(commit)).toBe(true);
      }),
    );
  });
});

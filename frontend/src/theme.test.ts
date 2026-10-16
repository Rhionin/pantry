import { describe, expect, it } from 'vitest';
import { theme } from './theme';

describe('theme', () => {
  it('uses the locked teal as the primary brand color', () => {
    expect(theme.primaryColor).toBe('teal');
    expect(theme.colors?.teal?.[6]).toBe('#0d9488');
    expect(theme.colors?.amber?.[6]).toBe('#f59e0b');
  });
});

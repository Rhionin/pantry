export const FAILED_NAME_CAP = 50;

export interface NameSummary {
  shown: string[];
  overflow: number;
}

// summarizeNames caps a failure or unknown list at 50 names and reports how
// many more were not shown.
export function summarizeNames(names: string[], cap = FAILED_NAME_CAP): NameSummary {
  if (names.length <= cap) {
    return { shown: names, overflow: 0 };
  }
  return { shown: names.slice(0, cap), overflow: names.length - cap };
}

export function provisionQuantity(entry: { quantity: number; adjustment?: number }): number {
  return entry.adjustment !== undefined ? entry.adjustment : entry.quantity;
}

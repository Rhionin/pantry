import { describe, expect, it } from 'vitest';
import type { MemberPackage } from './copy';
import { clampMonth, formatDuration, seesawView } from './seesaw';

const lemon: MemberPackage = {
  productId: 'lemon',
  name: 'Gatorade Lemon-Lime',
  onHand: 1,
  unitOfMeasure: 'canister',
  netAmount: 18.3,
  netUnit: 'oz',
};

describe('seesawView', () => {
  it('pins the amount and infers weeks from the rate', () => {
    const view = seesawView(
      { quantity: 48, dimension: 'mass', usage: { perMonth: 16, unit: 'oz' } },
      [lemon],
      3,
      null,
    );

    expect(view.caption).toBe('18.3 oz on hand · ≈ 5 wks');
    expect(view.rateLabel).toBe('at 16 oz / mo');
    expect(view.amount).toMatchObject({ label: '48 oz', pinned: true, isDefault: false });
    expect(view.time).toMatchObject({ label: '≈ 3 mo', pinned: false });
    expect(view.fillQuantity).toBe(48);
    expect(view.unknownHint).toBe('');
    expect(formatDuration(18.3 / 16)).toBe('5 wks');
  });

  it('leaves the other end blank until usage is known', () => {
    const amount = seesawView({ quantity: 48, dimension: 'mass' }, [lemon], 3, null);
    expect(amount.caption).toBe('18.3 oz on hand');
    expect(amount.time.label).toBe('—');
    expect(amount.unknownHint).toBe('Fills in once usage is known');
    expect(amount.fillQuantity).toBe(48);

    const time = seesawView({ windowMonths: 6 }, [lemon], 3, null);
    expect(time.time).toMatchObject({ label: '6 mo', pinned: true, isDefault: false });
    expect(time.amount.label).toBe('—');
    expect(time.fillQuantity).toBeUndefined();
  });

  it('marks the household window as the pinned default', () => {
    const view = seesawView({}, [lemon], 3, null);
    expect(view.time).toMatchObject({ label: '3 mo', pinned: true, isDefault: true, labelText: '3 mo, household default, pinned' });
    expect(view.amount.label).toBe('—');

    const unknown = seesawView({}, [], undefined, null);
    expect(unknown.time).toMatchObject({ label: 'Default', labelText: 'Household default, pinned' });
  });

  it('recomputes the free end while a draft is open', () => {
    const group = { quantity: 48, dimension: 'mass' as const, usage: { perMonth: 16, unit: 'oz' } };
    const edited = seesawView(group, [lemon], 3, { pin: 'time', value: 6 });
    expect(edited.time).toMatchObject({ label: '6 mo', pinned: true });
    expect(edited.amount.label).toBe('≈ 96 oz');
    expect(edited.fillQuantity).toBe(96);
    expect(clampMonth(18.3 / 16)).toBe(1);
  });

  it('uses fluid ounces when the group is volume', () => {
    const view = seesawView(
      { windowMonths: 2, dimension: 'volume', usage: { perMonth: 8, unit: 'fl oz' } },
      [],
      3,
      null,
    );
    expect(view.amount.label).toBe('≈ 16 fl oz');
    expect(view.rateLabel).toBe('at 8 fl oz / mo');
    expect(view.time.label).toBe('2 mo');
  });
});
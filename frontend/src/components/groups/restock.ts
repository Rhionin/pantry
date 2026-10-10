export const everyNoRestock = "Every product in this group is marked don't restock."

export interface RestockMember {
  productId: string;
  name: string;
  noRestock?: boolean;
  lastStockedAt?: string;
  lastConsumedAt?: string;
}

export interface RestockDeal {
  itemId?: string;
  productId?: string;
  priceCents?: number;
}

export interface RestockPick {
  productId: string;
  because: string;
}

function millis(value?: string): number | null {
  if (!value) return null;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? null : parsed;
}

function restockable<T extends { noRestock?: boolean }>(members: T[]): T[] {
  return members.filter((member) => !member.noRestock);
}

function byId<T extends { productId: string }>(members: T[], id?: string): T | undefined {
  if (!id) return undefined;
  return members.find((member) => member.productId === id);
}

function later(aTime: number, a: RestockMember, bTime: number, b: RestockMember): boolean {
  if (aTime !== bTime) return aTime > bTime;
  if (a.name !== b.name) return a.name < b.name;
  return a.productId < b.productId;
}

function pickSame(members: RestockMember[]): RestockPick {
  if (members.length === 0) return { productId: '', because: '' };
  const consumed = members.filter((member) => millis(member.lastConsumedAt) !== null);
  if (consumed.length > 0) {
    let best = consumed[0];
    for (const member of consumed.slice(1)) {
      if (later(millis(member.lastConsumedAt) ?? 0, member, millis(best.lastConsumedAt) ?? 0, best)) best = member;
    }
    return { productId: best.productId, because: `The last one used up was ${best.name}.` };
  }
  const stocked = members.filter((member) => millis(member.lastStockedAt) !== null);
  if (stocked.length > 0) {
    let best = stocked[0];
    for (const member of stocked.slice(1)) {
      if (later(millis(member.lastStockedAt) ?? 0, member, millis(best.lastStockedAt) ?? 0, best)) best = member;
    }
    return { productId: best.productId, because: 'Nothing has run out yet. This is the one you stocked last.' };
  }
  let best = members[0];
  for (const member of members.slice(1)) {
    if (member.productId < best.productId) best = member;
  }
  return { productId: best.productId, because: 'Nothing has run out yet.' };
}

function boughtEarlier(a: RestockMember, b: RestockMember): boolean {
  const aAt = millis(a.lastStockedAt);
  const bAt = millis(b.lastStockedAt);
  if ((aAt === null) !== (bAt === null)) return aAt === null;
  if (aAt !== null && bAt !== null && aAt !== bAt) return aAt < bAt;
  if (a.name !== b.name) return a.name < b.name;
  return a.productId < b.productId;
}

function variety(members: RestockMember[]): RestockPick {
  let best = members[0];
  for (const member of members.slice(1)) {
    if (boughtEarlier(member, best)) best = member;
  }
  return { productId: best.productId, because: `Next up: ${best.name} · rotates through ${members.length}` };
}

function dealPrice(member: RestockMember, deals: RestockDeal[]): number | null {
  const deal = deals.find((item) => (item.itemId !== undefined && item.itemId === member.productId) || item.productId === member.productId);
  if (!deal || deal.priceCents === undefined) return null;
  return deal.priceCents;
}

function money(cents: number): string {
  const safe = Math.max(0, Math.trunc(cents));
  return `$${Math.floor(safe / 100)}.${String(safe % 100).padStart(2, '0')}`;
}

function pickBest(members: RestockMember[], pin: string, deals: RestockDeal[]): RestockPick {
  const priced = members.flatMap((member) => {
    const price = dealPrice(member, deals);
    return price === null ? [] : [{ member, price }];
  });
  if (priced.length === 0) {
    const pinned = byId(members, pin);
    if (pinned) return { productId: pinned.productId, because: `Nothing is on sale. Otherwise buy ${pinned.name}.` };
    const same = pickSame(members);
    return { productId: same.productId, because: `Nothing is on sale and no fallback is set. ${same.because}` };
  }
  priced.sort((a, b) => {
    if (a.price !== b.price) return a.price - b.price;
    if (a.member.name !== b.member.name) return a.member.name < b.member.name ? -1 : 1;
    return a.member.productId < b.member.productId ? -1 : 1;
  });
  const winner = priced[0];
  return { productId: winner.member.productId, because: `On sale for ${money(winner.price)}.` };
}

// pickRestock chooses the product a rule would buy. Products marked don't
// restock stay in the group and never win. Price-per-ounce ranking for a
// sale stays on the server; this copy breaks a sale tie by the item price.
export function pickRestock(
  rule: string,
  members: RestockMember[],
  pin = '',
  deals: RestockDeal[] = [],
): RestockPick {
  if (members.length === 0) return { productId: '', because: '' };
  const eligible = restockable(members);
  if (eligible.length === 0) return { productId: '', because: everyNoRestock };
  if (rule === 'favor_variety') return variety(eligible);
  if (rule === 'favorite') {
    const pinned = byId(members, pin);
    if (pinned && !pinned.noRestock) return { productId: pinned.productId, because: 'This is the one with the star.' };
    const same = pickSame(eligible);
    if (pinned?.noRestock) return { productId: same.productId, because: `${pinned.name} isn't restocked. ${same.because}` };
    return { productId: same.productId, because: `No favorite is set. ${same.because}` };
  }
  if (rule === 'best_deal') {
    const pinned = byId(members, pin);
    const result = pickBest(eligible, pinned?.noRestock ? '' : pin, deals);
    const sales = eligible.some((member) => dealPrice(member, deals) !== null);
    if (pinned?.noRestock && !sales) {
      return { productId: result.productId, because: `${pinned.name} isn't restocked. ${result.because}` };
    }
    return result;
  }
  const full = pickSame(members);
  const result = pickSame(eligible);
  const skipped = byId(members, full.productId);
  if (skipped?.noRestock) return { productId: result.productId, because: `${skipped.name} isn't restocked. ${result.because}` };
  return result;
}

export function varietyLine(members: RestockMember[]): string {
  if (members.length === 0) return '';
  return pickRestock('favor_variety', members).because;
}

// restockNote is the line under the rule pill. Favor variety always names the
// next product. The other rules speak up only when the one they wanted is
// marked don't restock.
export function restockNote(
  group: { rule: string; ruleConfirmed: boolean; pinnedProductId?: string },
  members: RestockMember[],
): string {
  if (!group.ruleConfirmed || members.length === 0) return '';
  if (group.rule === 'favor_variety') return varietyLine(members);
  if (group.rule !== 'favorite' && group.rule !== 'same_as_ran_out') return '';
  const result = pickRestock(group.rule, members, group.pinnedProductId);
  if (result.because.includes("isn't restocked") || result.because === everyNoRestock) return result.because;
  return '';
}

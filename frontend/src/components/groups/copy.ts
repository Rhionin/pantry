import type { GroupMember, GroupTarget, NetDimension, ProductGroup, SuggestionMember, TargetConflictMember } from '../../types';

export const ruleLabels: Record<string, string> = {
  same_as_ran_out: 'Same as what ran out',
  favorite: 'Always my favorite',
  best_deal: 'Best deal',
};

export const ruleLabel = (rule: string) => ruleLabels[rule] ?? rule;

export const kindPhrase = (kind: string) => {
  if (kind === 'looks_alike') return 'look alike';
  if (kind === 'from_old_plan') return 'from the old shopping plan';
  if (kind === 'from_scan') return 'from a scan';
  return kind;
};

export const suggestionBanner = (cards: { kind: string }[]) => {
  if (cards.length === 0) return '';
  const counts = new Map<string, number>();
  for (const card of cards) {
    counts.set(card.kind, (counts.get(card.kind) ?? 0) + 1);
  }
  const parts = [...counts.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([kind, count]) => `${count} ${kindPhrase(kind)}`);
  const noun = cards.length === 1 ? 'suggestion' : 'suggestions';
  return `${cards.length} ${noun} to review. ${parts.join(', ')}.`;
};

export const targetLabel = (group: Pick<ProductGroup, 'quantity' | 'dimension' | 'windowMonths'>) => {
  if (group.quantity !== undefined) {
    return `${group.quantity} ${group.dimension === 'volume' ? 'fl oz' : 'oz'}`;
  }
  if (group.windowMonths !== undefined) return `${group.windowMonths} months`;
  return 'Household default';
};

export const onHandCount = (members: GroupMember[]) => members.reduce((sum, member) => sum + member.onHand, 0);

export function formatMemberSize(member: Pick<SuggestionMember, 'netAmount' | 'netUnit' | 'packCount'>): string {
  if (member.netAmount === undefined || member.netUnit === undefined || member.netUnit === '') return '';
  const amount = Number.isInteger(member.netAmount) ? String(member.netAmount) : String(member.netAmount);
  const size = `${amount} ${member.netUnit}`;
  if (member.packCount !== undefined && member.packCount > 1) return `${member.packCount} x ${size}`;
  return size;
}

export interface MemberFact {
  key: string;
  label: string;
  text: (member: SuggestionMember) => string;
}

// Rows that at least one member can fill. A blank cell stays in the row so
// the products still line up.
export function memberFacts(members: SuggestionMember[]): MemberFact[] {
  const defs: MemberFact[] = [
    { key: 'brand', label: 'Brand', text: (member) => member.brand ?? '' },
    { key: 'size', label: 'Size', text: (member) => formatMemberSize(member) },
    { key: 'variety', label: 'Variety', text: (member) => member.variety ?? '' },
    { key: 'package', label: 'Package', text: (member) => member.unitOfMeasure ?? '' },
    { key: 'barcode', label: 'Barcode', text: (member) => (member.barcodes ?? []).filter((code) => code !== '').join(', ') },
    { key: 'category', label: 'Category', text: (member) => member.category ?? '' },
  ];
  return defs.filter((fact) => members.some((member) => fact.text(member) !== ''));
}

export type GroupSupply = Pick<ProductGroup, 'quantity' | 'dimension' | 'windowMonths'>;

export interface TargetChoice {
  label: string;
  target: GroupTarget;
}

const quantityUnit = (dimension?: NetDimension) => (dimension === 'volume' ? 'fluid ounces' : 'ounces');

export const conflictAmount = (member: Pick<TargetConflictMember, 'quantity' | 'dimension' | 'windowMonths'>) => {
  if (member.quantity !== undefined) return `${member.quantity} ${quantityUnit(member.dimension)}`;
  if (member.windowMonths !== undefined) return `${member.windowMonths} months`;
  return '';
};

export const conflictLine = (member: TargetConflictMember) => {
  const amount = conflictAmount(member);
  if (amount === '') return member.name;
  return `${member.name}: ${amount}`;
};

const memberTarget = (member: TargetConflictMember): { key: string; amount: string; target: GroupTarget } | null => {
  if (member.quantity !== undefined) {
    const dimension = member.dimension === 'volume' || member.dimension === 'mass' ? member.dimension : undefined;
    const target: GroupTarget = { quantity: member.quantity };
    if (dimension) target.dimension = dimension;
    return { key: `quantity:${member.quantity}:${dimension ?? ''}`, amount: conflictAmount(member), target };
  }
  if (member.windowMonths !== undefined) {
    return {
      key: `window:${member.windowMonths}`,
      amount: conflictAmount(member),
      target: { windowMonths: member.windowMonths },
    };
  }
  return null;
};

// Clear would wipe a group's own target. It is only the household default.
export const targetChoices = (group: GroupSupply | null, members: TargetConflictMember[]): TargetChoice[] => {
  const choices: TargetChoice[] = [];
  if (group?.quantity !== undefined) {
    const target: GroupTarget = { quantity: group.quantity };
    if (group.dimension === 'mass' || group.dimension === 'volume') target.dimension = group.dimension;
    choices.push({ label: `Keep the group's ${targetLabel(group)}`, target });
  } else if (group?.windowMonths !== undefined) {
    choices.push({ label: `Keep the group's ${targetLabel(group)}`, target: { windowMonths: group.windowMonths } });
  } else {
    choices.push({ label: 'Keep the household default', target: { clear: true } });
  }

  const seen = new Map<string, { amount: string; target: GroupTarget; names: string[] }>();
  for (const member of members) {
    const built = memberTarget(member);
    if (!built) continue;
    const existing = seen.get(built.key);
    if (existing) {
      existing.names.push(member.name);
      continue;
    }
    seen.set(built.key, { amount: built.amount, target: built.target, names: [member.name] });
  }
  for (const choice of seen.values()) {
    choices.push({
      label: `Use ${choice.amount} (from ${choice.names.join(', ')})`,
      target: choice.target,
    });
  }
  return choices;
};

export type GroupChip = 'all' | 'low' | 'unconfirmed' | string;

export const matchesChip = (group: ProductGroup, chip: GroupChip) => {
  if (chip === 'all') return true;
  if (chip === 'low') return group.runningLow;
  if (chip === 'unconfirmed') return !group.ruleConfirmed;
  return group.rule === chip;
};

export const matchesSearch = (group: ProductGroup, query: string) => {
  const q = query.trim().toLowerCase();
  if (q === '') return true;
  if (group.name.toLowerCase().includes(q)) return true;
  return group.members.some((member) => member.name.toLowerCase().includes(q));
};

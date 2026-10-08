import type { GroupMember, ProductGroup, TargetConflictMember } from '../../types';

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
  return 'Account window';
};

export const onHandCount = (members: GroupMember[]) => members.reduce((sum, member) => sum + member.onHand, 0);

export const conflictLine = (member: TargetConflictMember) => {
  if (member.quantity !== undefined) return `${member.name}: keep ${member.quantity}`;
  if (member.windowMonths !== undefined) return `${member.name}: ${member.windowMonths} months`;
  return member.name;
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

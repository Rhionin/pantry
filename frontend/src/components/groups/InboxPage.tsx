import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  Alert, Anchor, Button, Checkbox, Group, Loader, NumberInput, Stack, Text, TextInput, Title,
} from '@mantine/core';
import { ApiError, acceptSuggestion, dismissSuggestion, listSuggestions, skipSuggestion } from '../../api/client';
import type { GroupSuggestion, GroupTarget, TargetConflictMember } from '../../types';
import { conflictLine } from './copy';

export const InboxPage = () => {
  const [cards, setCards] = useState<GroupSuggestion[]>([]);
  const [index, setIndex] = useState(0);
  const [checks, setChecks] = useState<{ cardId: string; values: Record<string, boolean> } | null>(null);
  const [editingId, setEditingId] = useState('');
  const [nameDraft, setNameDraft] = useState<{ cardId: string; name: string } | null>(null);
  const [conflictFor, setConflictFor] = useState<{ cardId: string; members: TargetConflictMember[] } | null>(null);
  const [months, setMonths] = useState<number | string>(3);
  const [ounces, setOunces] = useState<number | string>('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setError('');
    try {
      const rows = await listSuggestions();
      setCards(rows);
      setIndex((current) => (rows.length === 0 ? 0 : Math.min(current, rows.length - 1)));
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to load suggestions.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  const card = cards[index];
  const checked = checks !== null && card !== undefined && checks.cardId === card.id
    ? checks.values
    : Object.fromEntries((card?.members ?? []).map((member) => [member.productId, member.included]));
  const name = nameDraft !== null && card !== undefined && nameDraft.cardId === card.id ? nameDraft.name : (card?.title ?? '');
  const editing = card !== undefined && editingId === card.id;
  const conflict = conflictFor !== null && card !== undefined && conflictFor.cardId === card.id ? conflictFor.members : null;

  const setChecked = (productId: string, on: boolean) => {
    if (!card) return;
    setChecks({ cardId: card.id, values: { ...checked, [productId]: on } });
  };

  const selected = card ? card.members.filter((member) => checked[member.productId]).map((member) => member.productId) : [];

  const group = async (target?: GroupTarget) => {
    if (!card || selected.length === 0) return;
    setSaving(true);
    setError('');
    try {
      const renamed = name.trim() !== '' && name.trim() !== card.title ? name.trim() : undefined;
      await acceptSuggestion(card.id, selected, renamed, target);
      setConflictFor(null);
      await load();
    } catch (requestError) {
      if (requestError instanceof ApiError && requestError.code === 'target_decision_required') {
        setConflictFor({ cardId: card.id, members: requestError.members });
        return;
      }
      setError(requestError instanceof Error ? requestError.message : 'Unable to group these products.');
    } finally {
      setSaving(false);
    }
  };

  const dismiss = async () => {
    if (!card) return;
    setSaving(true);
    setError('');
    try {
      await dismissSuggestion(card.id);
      await load();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to dismiss that suggestion.');
    } finally {
      setSaving(false);
    }
  };

  const skip = async () => {
    if (!card) return;
    setError('');
    try {
      await skipSuggestion(card.id);
      setIndex((current) => (cards.length <= 1 ? 0 : (current + 1) % cards.length));
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to skip that suggestion.');
    }
  };

  return (
    <Stack gap="sm">
      <Anchor component={Link} to="/groups" size="sm">Product groups</Anchor>
      <Title order={1} size="h3">Suggestions</Title>
      {loading && <Loader aria-label="Loading suggestions" />}
      {error !== '' && <Alert color="red" py="xs">{error}</Alert>}
      {!loading && cards.length === 0 && <Text c="dimmed">Nothing to review.</Text>}
      {card && (
        <Stack gap="xs">
          <Text size="sm" c="dimmed">{index + 1} of {cards.length}</Text>
          <Title order={2} size="h4">{card.title}</Title>
          {card.members.map((member) => (
            <Stack key={member.productId} gap={2}>
              <Checkbox
                label={member.name}
                checked={checked[member.productId] === true}
                onChange={(event) => setChecked(member.productId, event.currentTarget.checked)}
              />
              {member.caution !== '' && <Text size="sm" c="dimmed">{member.caution}</Text>}
            </Stack>
          ))}
          {editing && (
            <TextInput
              label="Group name"
              value={name}
              onChange={(event) => card && setNameDraft({ cardId: card.id, name: event.currentTarget.value })}
            />
          )}
          {conflict !== null && (
            <Alert color="yellow" title="Choose what this group should keep on hand">
              <Stack gap="xs">
                {conflict.map((member) => <Text key={member.productId} size="sm">{conflictLine(member)}</Text>)}
                <Text size="sm">These products already have their own supply setting.</Text>
                <NumberInput label="Ounces" min={0} value={ounces} onChange={setOunces} w={140} />
                <NumberInput label="Months" min={1} max={12} allowDecimal={false} value={months} onChange={setMonths} w={140} />
                <Group gap="xs">
                  <Button size="xs" onClick={() => void group({ clear: true })}>Use the account window</Button>
                  <Button
                    size="xs"
                    variant="light"
                    onClick={() => {
                      const value = typeof ounces === 'number' ? ounces : Number(ounces);
                      if (!Number.isFinite(value) || value <= 0) return;
                      void group({ quantity: value, dimension: 'mass' });
                    }}
                  >
                    Use ounces
                  </Button>
                  <Button
                    size="xs"
                    variant="light"
                    onClick={() => {
                      const value = typeof months === 'number' ? months : Number(months);
                      if (!Number.isInteger(value) || value < 1 || value > 12) return;
                      void group({ windowMonths: value });
                    }}
                  >
                    Use months
                  </Button>
                </Group>
              </Stack>
            </Alert>
          )}
          <Group gap="xs">
            <Button size="xs" loading={saving} disabled={selected.length === 0} onClick={() => void group()}>
              {card.existingGroupId ? 'Add to group' : 'Group'}
            </Button>
            <Button size="xs" variant="light" onClick={() => card && setEditingId(card.id)}>Edit</Button>
            <Button size="xs" variant="default" loading={saving} onClick={() => void dismiss()}>Not the same</Button>
            <Button size="xs" variant="subtle" onClick={() => void skip()}>Skip for now</Button>
          </Group>
        </Stack>
      )}
    </Stack>
  );
};

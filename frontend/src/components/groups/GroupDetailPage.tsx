import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  Alert, Anchor, Badge, Button, Group, Loader, NativeSelect, NumberInput, Stack, Text, TextInput, Title,
} from '@mantine/core';
import {
  addGroupMembers, getGroup, getInventoryList, getProduct, listGroups, listProducts, removeGroupMember, renameGroup, setGroupTarget,
} from '../../api/client';
import type { ProductGroup } from '../../types';
import { RuleSheet } from '../inventory/RuleSheet';
import { AddToGroup } from './AddToGroup';
import { ungroupedProducts, type AddCandidate } from './candidates';
import { onHandCount, ruleLabel, targetLabel } from './copy';

export const GroupDetailPage = () => {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const [group, setGroup] = useState<ProductGroup | null>(null);
  const [others, setOthers] = useState<ProductGroup[]>([]);
  const [barcodes, setBarcodes] = useState<Record<string, string[]>>({});
  const [name, setName] = useState('');
  const [moveTo, setMoveTo] = useState('');
  const [moving, setMoving] = useState('');
  const [months, setMonths] = useState<number | string>('');
  const [ounces, setOunces] = useState<number | string>('');
  const [candidates, setCandidates] = useState<AddCandidate[]>([]);
  const [ruleOpen, setRuleOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setError('');
    try {
      const [view, all, products, inventory] = await Promise.all([
        getGroup(id), listGroups(), listProducts(), getInventoryList(),
      ]);
      setGroup(view);
      setName(view.name);
      setOthers(all.filter((item) => item.id !== view.id));
      setMonths(view.windowMonths ?? '');
      setOunces(view.quantity ?? '');
      setCandidates(ungroupedProducts(products, inventory, [view, ...all]));
      const codes: Record<string, string[]> = {};
      await Promise.all(view.members.map(async (member) => {
        try {
          const product = await getProduct(member.productId);
          codes[member.productId] = product.barcodes ?? [];
        } catch {
          codes[member.productId] = [];
        }
      }));
      setBarcodes(codes);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to load that group.');
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  const rename = async () => {
    if (!group || name.trim() === '') return;
    setError('');
    try {
      setGroup(await renameGroup(group.id, name.trim()));
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to rename the group.');
    }
  };

  const remove = async (productId: string) => {
    if (!group) return;
    setError('');
    try {
      const result = await removeGroupMember(group.id, productId);
      if (result.deleted) {
        navigate('/groups');
        return;
      }
      await load();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to remove that product.');
    }
  };

  const move = async (productId: string) => {
    if (moveTo === '') return;
    setMoving(productId);
    setError('');
    try {
      await addGroupMembers(moveTo, [productId], id);
      await load();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to move that product.');
    } finally {
      setMoving('');
    }
  };

  const saveTarget = async (body: { clear?: boolean; windowMonths?: number; quantity?: number; dimension?: 'mass' }) => {
    if (!group) return;
    setError('');
    try {
      setGroup(await setGroupTarget(group.id, body));
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to save the target.');
    }
  };

  if (loading) return <Loader aria-label="Loading group" />;

  return (
    <Stack gap="sm">
      <Anchor component={Link} to="/groups" size="sm">Product groups</Anchor>
      {error !== '' && <Alert color="red" py="xs">{error}</Alert>}
      {group && (
        <>
          <Title order={1} size="h3">{group.name}</Title>
          <Text size="sm">{onHandCount(group.members)} on hand · {targetLabel(group)}</Text>
          <Badge
            component="button"
            type="button"
            variant="light"
            color={group.ruleConfirmed ? undefined : 'yellow'}
            w="fit-content"
            className="rule-badge-button"
            aria-label={group.ruleConfirmed ? `Edit rule: ${ruleLabel(group.rule)}` : 'Pick a rule'}
            onClick={() => setRuleOpen(true)}
          >
            {group.ruleConfirmed ? ruleLabel(group.rule) : 'Pick a rule'}
          </Badge>
          <Group align="end" gap="xs">
            <TextInput label="Name" value={name} onChange={(event) => setName(event.currentTarget.value)} style={{ flex: 1 }} />
            <Button size="xs" onClick={() => void rename()}>Rename</Button>
          </Group>
          <Stack gap="xs">
            <Text fw={600}>Keep on hand</Text>
            <Group align="end" gap="xs">
              <NumberInput label="Ounces" value={ounces} onChange={setOunces} w={120} />
              <Button size="xs" variant="light" onClick={() => {
                const value = typeof ounces === 'number' ? ounces : Number(ounces);
                if (!Number.isFinite(value) || value <= 0) return;
                void saveTarget({ quantity: value, dimension: 'mass' });
              }}>Save ounces</Button>
            </Group>
            <Group align="end" gap="xs">
              <NumberInput label="Months" min={1} max={12} allowDecimal={false} value={months} onChange={setMonths} w={120} />
              <Button size="xs" variant="light" onClick={() => {
                const value = typeof months === 'number' ? months : Number(months);
                if (!Number.isInteger(value)) return;
                void saveTarget({ windowMonths: value });
              }}>Save months</Button>
              <Button size="xs" variant="default" onClick={() => void saveTarget({ clear: true })}>Use the account window</Button>
            </Group>
          </Stack>
          <AddToGroup groupId={group.id} group={group} candidates={candidates} onAdded={() => { void load(); }} />
          {group.members.map((member) => (
            <Stack key={member.productId} gap={4}>
              <Text fw={600}>{member.name}</Text>
              <Text size="sm">{member.onHand} on hand</Text>
              {(barcodes[member.productId] ?? []).length > 0 && (
                <Text size="xs" c="dimmed">Barcode: {(barcodes[member.productId] ?? []).join(', ')}</Text>
              )}
              <Group gap="xs">
                <Button size="xs" variant="default" onClick={() => void remove(member.productId)}>Remove</Button>
                {others.length > 0 && (
                  <>
                    <NativeSelect
                      aria-label={`Move ${member.name}`}
                      value={moveTo}
                      onChange={(event) => setMoveTo(event.currentTarget.value)}
                      data={[{ value: '', label: 'Move to…' }, ...others.map((item) => ({ value: item.id, label: item.name }))]}
                    />
                    <Button size="xs" variant="light" loading={moving === member.productId} disabled={moveTo === ''} onClick={() => void move(member.productId)}>
                      Move
                    </Button>
                  </>
                )}
              </Group>
            </Stack>
          ))}
          {group.members.length === 0 && <Text c="dimmed">This group has no products yet.</Text>}
          {ruleOpen && (
            <RuleSheet
              group={group}
              opened
              onClose={() => setRuleOpen(false)}
              onSaved={() => { void load(); }}
            />
          )}
        </>
      )}
    </Stack>
  );
};

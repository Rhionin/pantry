import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  Alert, Anchor, Button, Group, Loader, Modal, NativeSelect, NumberInput, Stack, Text, TextInput,
} from '@mantine/core';
import {
  addGroupMembers, getGroup, getInventoryList, getProduct, getSupplySettings, listGroups, listProducts, removeGroupMember, renameGroup, setGroupTarget,
} from '../../api/client';
import type { GroupTarget, ProductDetail, ProductGroup } from '../../types';
import { RuleSheet } from '../inventory/RuleSheet';
import { AddToGroup } from './AddToGroup';
import { BinMark } from './BinMark';
import { ungroupedProducts, type AddCandidate } from './candidates';
import {
  binColor, binView, detailStockSentence, memberLine, memberOrder, memberPackage, ruleSentence, type MemberPackage,
} from './copy';

const closeButton = { 'aria-label': 'Close' };

export const GroupDetailPage = () => {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const [group, setGroup] = useState<ProductGroup | null>(null);
  const [others, setOthers] = useState<ProductGroup[]>([]);
  const [details, setDetails] = useState<Record<string, Partial<ProductDetail>>>({});
  const [householdMonths, setHouseholdMonths] = useState<number | undefined>();
  const [name, setName] = useState('');
  const [renaming, setRenaming] = useState(false);
  const [moveTo, setMoveTo] = useState('');
  const [moving, setMoving] = useState('');
  const [actingOn, setActingOn] = useState('');
  const [months, setMonths] = useState<number | string>('');
  const [ounces, setOunces] = useState<number | string>('');
  const [targetOpen, setTargetOpen] = useState(false);
  const [adding, setAdding] = useState(false);
  const [candidates, setCandidates] = useState<AddCandidate[]>([]);
  const [ruleOpen, setRuleOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setError('');
    try {
      const [view, all, products, inventory, settings] = await Promise.all([
        getGroup(id),
        listGroups(),
        listProducts(),
        getInventoryList(),
        getSupplySettings().catch(() => null),
      ]);
      setGroup(view);
      setName(view.name);
      setOthers(all.filter((item) => item.id !== view.id));
      setHouseholdMonths(settings?.months);
      setCandidates(ungroupedProducts(products, inventory, [view, ...all]));
      const loaded: Record<string, Partial<ProductDetail>> = {};
      await Promise.all(view.members.map(async (member) => {
        try {
          loaded[member.productId] = await getProduct(member.productId);
        } catch {
          loaded[member.productId] = { barcodes: [] };
        }
      }));
      setDetails(loaded);
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
      setRenaming(false);
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
      setActingOn('');
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
      setActingOn('');
      setMoveTo('');
      await load();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to move that product.');
    } finally {
      setMoving('');
    }
  };

  const saveTarget = async (body: GroupTarget) => {
    if (!group) return;
    setError('');
    try {
      setGroup(await setGroupTarget(group.id, body));
      setTargetOpen(false);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to save the target.');
    }
  };

  const openTarget = () => {
    if (!group) return;
    setOunces(group.quantity ?? '');
    setMonths(group.windowMonths ?? '');
    setError('');
    setTargetOpen(true);
  };

  if (loading) return <Loader aria-label="Loading group" />;

  const members: MemberPackage[] = memberOrder((group?.members ?? []).map((member) => (
    memberPackage(member, details[member.productId])
  )));
  const view = group ? binView(group, members) : null;
  const acting = members.find((member) => member.productId === actingOn);

  return (
    <Stack gap="lg" className="bin-page">
      <Anchor component={Link} to="/groups" size="sm">Product groups</Anchor>
      {error !== '' && !renaming && !targetOpen && actingOn === '' && <Alert color="red" py="xs">{error}</Alert>}
      {group && view && (
        <>
          <div className="bin-hero-row">
            <BinMark
              view={view}
              members={members}
              size="hero"
              name={group.name}
              onRename={() => {
                setName(group.name);
                setError('');
                setRenaming(true);
              }}
            />
            <div className="bin-copy">
              <div className="bin-block">
                <Text className="bin-stock">{detailStockSentence(group, members, householdMonths)}</Text>
                <Anchor component="button" type="button" className="bin-action" onClick={openTarget}>
                  Change target
                </Anchor>
              </div>
              <div className="bin-block">
                <Text className="bin-rule">{ruleSentence(group, members, 'detail')}</Text>
                <Anchor component="button" type="button" className="bin-action" onClick={() => setRuleOpen(true)}>
                  {group.ruleConfirmed ? 'Change rule' : 'Pick a rule'}
                </Anchor>
              </div>
            </div>
          </div>
          <div className="bin-members">
            {members.map((member) => (
              <article key={member.productId} className="bin-member">
                <div className="bin-member-side">
                  <span className="bin-swatch" style={{ background: binColor(members, member.productId) }} aria-hidden="true" />
                  <Anchor
                    component="button"
                    type="button"
                    className="bin-action bin-member-action"
                    aria-label={`Remove or move ${member.name}`}
                    onClick={() => {
                      setMoveTo('');
                      setError('');
                      setActingOn(member.productId);
                    }}
                  >
                    Remove or move
                  </Anchor>
                </div>
                <div className="bin-member-copy">
                  <p className="bin-member-name">{member.name}</p>
                  <Text size="sm" c="dimmed">{memberLine(member)}</Text>
                  {(member.barcodes ?? []).length > 0 && (
                    <Text size="xs" c="dimmed">Barcode: {(member.barcodes ?? []).join(', ')}</Text>
                  )}
                </div>
              </article>
            ))}
            {members.length === 0 && <Text c="dimmed">This group has no products yet.</Text>}
          </div>
          <Button color="dark" w="fit-content" onClick={() => setAdding(true)}>Add a product</Button>
          <Modal
            opened={renaming}
            onClose={() => setRenaming(false)}
            title="Rename"
            closeButtonProps={closeButton}
          >
            <Stack gap="sm">
              <TextInput label="Name" value={name} onChange={(event) => setName(event.currentTarget.value)} />
              {error !== '' && <Text size="sm" c="red">{error}</Text>}
              <Button onClick={() => void rename()} disabled={name.trim() === ''}>Rename</Button>
            </Stack>
          </Modal>
          <Modal
            opened={targetOpen}
            onClose={() => setTargetOpen(false)}
            title="Change target"
            closeButtonProps={closeButton}
          >
            <Stack gap="sm">
              <Text size="sm">Choose ounces, a number of months, or the household default.</Text>
              <Group align="end" gap="xs">
                <NumberInput
                  label={group.dimension === 'volume' ? 'Fluid ounces' : 'Ounces'}
                  value={ounces}
                  onChange={setOunces}
                  w={140}
                />
                <Button size="xs" onClick={() => {
                  const value = typeof ounces === 'number' ? ounces : Number(ounces);
                  if (!Number.isFinite(value) || value <= 0) {
                    setError('Enter an amount greater than zero.');
                    return;
                  }
                  void saveTarget({ quantity: value, dimension: group.dimension === 'volume' ? 'volume' : 'mass' });
                }}>Save ounces</Button>
              </Group>
              <Group align="end" gap="xs">
                <NumberInput
                  label="Months"
                  min={1}
                  max={12}
                  allowDecimal={false}
                  value={months}
                  onChange={setMonths}
                  w={140}
                />
                <Button size="xs" variant="light" onClick={() => {
                  const value = typeof months === 'number' ? months : Number(months);
                  if (!Number.isInteger(value) || value < 1 || value > 12) {
                    setError('Enter a whole number of months from 1 to 12.');
                    return;
                  }
                  void saveTarget({ windowMonths: value });
                }}>Save months</Button>
              </Group>
              <Button size="xs" variant="default" onClick={() => void saveTarget({ clear: true })}>
                Use the household default
              </Button>
              {error !== '' && <Text size="sm" c="red">{error}</Text>}
            </Stack>
          </Modal>
          <Modal
            opened={acting !== undefined}
            onClose={() => setActingOn('')}
            title="Remove or move"
            closeButtonProps={closeButton}
          >
            {acting && (
              <Stack gap="sm">
                <Text fw={650}>{acting.name}</Text>
                <Button variant="default" onClick={() => void remove(acting.productId)}>Remove</Button>
                {others.length > 0 && (
                  <Group align="end" gap="xs">
                    <NativeSelect
                      aria-label={`Move ${acting.name}`}
                      value={moveTo}
                      onChange={(event) => setMoveTo(event.currentTarget.value)}
                      data={[{ value: '', label: 'Move to…' }, ...others.map((item) => ({ value: item.id, label: item.name }))]}
                    />
                    <Button
                      size="xs"
                      variant="light"
                      loading={moving === acting.productId}
                      disabled={moveTo === ''}
                      onClick={() => void move(acting.productId)}
                    >
                      Move
                    </Button>
                  </Group>
                )}
                {error !== '' && <Text size="sm" c="red">{error}</Text>}
              </Stack>
            )}
          </Modal>
          <Modal
            opened={adding}
            onClose={() => setAdding(false)}
            title="Add a product"
            closeButtonProps={closeButton}
          >
            {adding && (
              <AddToGroup
                groupId={group.id}
                group={group}
                candidates={candidates}
                showHeading={false}
                onAdded={() => {
                  setAdding(false);
                  void load();
                }}
              />
            )}
          </Modal>
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

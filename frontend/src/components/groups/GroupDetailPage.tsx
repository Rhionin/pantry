import { useCallback, useEffect, useState, type CSSProperties } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ActionIcon, Alert, Anchor, Button, Loader, Menu, Modal, Stack, Text, TextInput, Tooltip,
} from '@mantine/core';
import {
  addGroupMembers, getGroup, getInventoryList, getProduct, getSupplySettings, listGroups, listProducts, removeGroupMember, renameGroup, setGroupTarget,
} from '../../api/client';
import type { GroupTarget, ProductDetail, ProductGroup } from '../../types';
import { AddToGroup } from './AddToGroup';
import { BinMark } from './BinMark';
import { ungroupedProducts, type AddCandidate } from './candidates';
import {
  binColor, binView, memberLine, memberOrder, memberPackage, onHandOunces, rulePillLabel, ruleSentence, type MemberPackage,
} from './copy';
import { RulePicker } from './RulePicker';
import { Seesaw } from './Seesaw';
import { seesawView, stockMeter, type SeesawDraft } from './seesaw';

const closeButton = { 'aria-label': 'Close' };

const iconInk = {
  '--ai-color': 'var(--bin-ink)',
  '--ai-hover-color': 'var(--bin-ink)',
  color: 'var(--bin-ink)',
} as CSSProperties;

function KebabIcon() {
  return (
    <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true">
      <circle cx="8" cy="3.1" r="1.35" fill="currentColor" />
      <circle cx="8" cy="8" r="1.35" fill="currentColor" />
      <circle cx="8" cy="12.9" r="1.35" fill="currentColor" />
    </svg>
  );
}

export const GroupDetailPage = () => {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const [group, setGroup] = useState<ProductGroup | null>(null);
  const [others, setOthers] = useState<ProductGroup[]>([]);
  const [details, setDetails] = useState<Record<string, Partial<ProductDetail>>>({});
  const [householdMonths, setHouseholdMonths] = useState<number | undefined>();
  const [name, setName] = useState('');
  const [renaming, setRenaming] = useState(false);
  const [moving, setMoving] = useState('');
  const [draft, setDraft] = useState<SeesawDraft | null>(null);
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
      await load();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to remove that product.');
    }
  };

  const move = async (productId: string, destination: string) => {
    setMoving(productId);
    setError('');
    try {
      await addGroupMembers(destination, [productId], id);
      await load();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to move that product.');
    } finally {
      setMoving('');
    }
  };

  const saveTarget = async (body: GroupTarget) => {
    if (!group) return;
    setGroup(await setGroupTarget(group.id, body));
  };

  if (loading) return <Loader aria-label="Loading group" />;

  const members: MemberPackage[] = memberOrder((group?.members ?? []).map((member) => (
    memberPackage(member, details[member.productId])
  )));
  const shown = group ? seesawView(group, members, householdMonths, draft) : null;
  const view = group && shown
    ? binView(
      shown.fillQuantity !== undefined
        ? { quantity: shown.fillQuantity, dimension: shown.dimension }
        : { windowMonths: shown.time.pinned ? group.windowMonths : undefined },
      members,
    )
    : null;
  const ruleName = group ? rulePillLabel(group, members) : '';
  const ruleDetail = group ? ruleSentence(group, members, 'detail') : '';
  const meter = shown ? stockMeter(shown, onHandOunces(members, shown.dimension)) : null;

  return (
    <Stack gap="lg" className="bin-page">
      <Anchor component={Link} to="/groups" size="sm">Product groups</Anchor>
      {error !== '' && !renaming && <Alert color="red" py="xs">{error}</Alert>}
      {group && view && shown && (
        <>
          <div className="bin-hero-row">
            <BinMark
              view={view}
              members={members}
              size="hero"
              name={group.name}
              caption={shown.caption}
              meter={meter ?? undefined}
              onRename={() => {
                setName(group.name);
                setError('');
                setRenaming(true);
              }}
            />
            <div className="seesaw-slot">
              <Seesaw
                view={shown}
                group={group}
                draft={draft}
                onDraft={setDraft}
                onSave={saveTarget}
              />
            </div>
            <div className="bin-copy">
              <Tooltip label={ruleDetail}>
                <button
                  type="button"
                  className="rule-pill"
                  aria-label={group.ruleConfirmed ? `Change rule, ${ruleName}` : ruleName}
                  onClick={() => setRuleOpen(true)}
                >
                  <span className="rule-pill-mark" aria-hidden="true">⇄</span>
                  <span className="rule-pill-label">{ruleName}</span>
                </button>
              </Tooltip>
            </div>
          </div>
          <div className="bin-members">
            {members.map((member) => (
              <article key={member.productId} className="bin-member">
                <span className="bin-swatch" style={{ background: binColor(members, member.productId) }} aria-hidden="true" />
                <div className="bin-member-copy">
                  <p className="bin-member-name">{member.name}</p>
                  <Text size="sm" c="dimmed">{memberLine(member)}</Text>
                  {(member.barcodes ?? []).length > 0 && (
                    <Text size="xs" c="dimmed">Barcode: {(member.barcodes ?? []).join(', ')}</Text>
                  )}
                </div>
                <Menu position="bottom-end">
                  <Menu.Target>
                    <Tooltip label="Remove or move">
                      <ActionIcon
                        variant="subtle"
                        color="dark"
                        className="bin-icon-button"
                        style={iconInk}
                        aria-label={`Remove or move ${member.name}`}
                      >
                        <KebabIcon />
                      </ActionIcon>
                    </Tooltip>
                  </Menu.Target>
                  <Menu.Dropdown>
                    <Menu.Item onClick={() => void remove(member.productId)}>Remove</Menu.Item>
                    {others.length > 0 && <Menu.Label>Move to</Menu.Label>}
                    {others.map((item) => (
                      <Menu.Item
                        key={item.id}
                        disabled={moving === member.productId}
                        onClick={() => void move(member.productId, item.id)}
                      >
                        {item.name}
                      </Menu.Item>
                    ))}
                  </Menu.Dropdown>
                </Menu>
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
            <RulePicker
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

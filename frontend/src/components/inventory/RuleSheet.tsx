import { useEffect, useState } from 'react';
import { Button, Drawer, Select, Stack, Text, TextInput } from '@mantine/core';
import { previewGroup, setGroupRule, setGroupTarget } from '../../api/client';
import type { GroupTarget, NetDimension } from '../../types';
import { ruleLabel, ruleLabels, targetLabel } from '../groups/copy';

// The fields the drawer reads. Inventory groups and product groups both satisfy this.
export interface RuleSheetGroup {
  id: string;
  name: string;
  rule: string;
  ruleConfirmed: boolean;
  pinnedProductId?: string;
  windowMonths?: number;
  quantity?: number;
  dimension?: NetDimension;
  members: { productId: string; name: string }[];
}

export interface RuleSheetProps {
  group: RuleSheetGroup;
  opened: boolean;
  onClose: () => void;
  onSaved: () => void;
}

const ruleOptions = Object.entries(ruleLabels).map(([value, label]) => ({ value, label }));

export const RuleSheet = ({ group, opened, onClose, onSaved }: RuleSheetProps) => {
  const [rule, setRule] = useState(group.rule);
  const [pin, setPin] = useState(group.pinnedProductId ?? '');
  const [targetKind, setTargetKind] = useState(group.quantity !== undefined ? 'ounces' : group.windowMonths !== undefined ? 'months' : 'account');
  const [amount, setAmount] = useState(group.quantity !== undefined ? String(group.quantity) : group.windowMonths !== undefined ? String(group.windowMonths) : '');
  const [preview, setPreview] = useState('');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!opened) return;
    let active = true;
    void previewGroup({ groupId: group.id, rule, pinnedProductId: pin })
      .then((result) => {
        if (!active) return;
        const sentence = result.explain || result.because;
        setPreview(sentence);
      })
      .catch(() => {
        if (active) setPreview('');
      });
    return () => {
      active = false;
    };
  }, [opened, group.id, rule, pin]);

  const memberOptions = group.members.map((member) => ({ value: member.productId, label: member.name }));
  const needsPin = rule === 'favorite' || rule === 'best_deal';

  const save = async () => {
    setSaving(true);
    setError('');
    try {
      await setGroupRule(group.id, rule, needsPin ? pin : '', true);
      let target: GroupTarget = { clear: true };
      if (targetKind === 'ounces') {
        const quantity = Number(amount);
        if (!(quantity > 0) || quantity > 999) {
          setError('The quantity has to be more than zero and at most 999 ounces.');
          return;
        }
        target = { quantity, dimension: 'mass' };
      } else if (targetKind === 'months') {
        const windowMonths = Number(amount);
        if (!Number.isInteger(windowMonths) || windowMonths < 1 || windowMonths > 12) {
          setError('The supply window has to be between 1 and 12 months.');
          return;
        }
        target = { windowMonths };
      }
      await setGroupTarget(group.id, target);
      onSaved();
      onClose();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to save the rule.');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Drawer opened={opened} onClose={onClose} position="bottom" title={group.name} size="auto">
      <Stack gap="sm" pb="md">
        {!group.ruleConfirmed && (
          <Text size="sm">
            This is still {ruleLabel(group.rule)}. Pick a rule so the next trip is yours.
          </Text>
        )}
        <Select label="Rule" data={ruleOptions} value={rule} onChange={(value) => value && setRule(value)} allowDeselect={false} />
        {needsPin && (
          <Select
            label={rule === 'favorite' ? 'Always buy' : 'Otherwise buy'}
            data={memberOptions}
            value={pin}
            onChange={(value) => setPin(value ?? '')}
            placeholder="Choose a product"
          />
        )}
        <Select
          label="Keep on hand"
          data={[
            { value: 'account', label: 'Household default' },
            { value: 'ounces', label: 'Ounces' },
            { value: 'months', label: 'Months' },
          ]}
          value={targetKind}
          onChange={(value) => value && setTargetKind(value)}
          allowDeselect={false}
        />
        {targetKind !== 'account' && (
          <TextInput
            label={targetKind === 'ounces' ? 'Ounces' : 'Months'}
            value={amount}
            onChange={(event) => setAmount(event.currentTarget.value)}
          />
        )}
        <Text size="sm">Now: {targetLabel({ quantity: group.quantity, dimension: group.dimension, windowMonths: group.windowMonths })}</Text>
        {preview !== '' && <Text size="sm">Next trip: {preview}</Text>}
        {error !== '' && <Text size="sm" c="red">{error}</Text>}
        <Button onClick={() => void save()} loading={saving}>Save rule</Button>
      </Stack>
    </Drawer>
  );
};

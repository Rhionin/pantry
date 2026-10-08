import { Select, Stack, TextInput } from '@mantine/core';
import { sizeUnits, type NetSizeDraft } from './netSize';

export interface NetSizeFieldsProps {
  draft: NetSizeDraft;
  onChange: (draft: NetSizeDraft) => void;
}

export const NetSizeFields = ({ draft, onChange }: NetSizeFieldsProps) => (
  <Stack gap="xs">
    <TextInput
      size="xs"
      label="Size"
      inputMode="decimal"
      value={draft.amount}
      onChange={(event) => onChange({ ...draft, amount: event.currentTarget.value })}
    />
    <Select
      size="xs"
      label="Unit"
      data={sizeUnits}
      value={draft.unit === '' ? null : draft.unit}
      onChange={(value) => onChange({ ...draft, unit: value ?? '' })}
      allowDeselect={false}
    />
    <TextInput
      size="xs"
      label="Package"
      description="The word on the shelf, such as can or jar."
      value={draft.packageWord}
      onChange={(event) => onChange({ ...draft, packageWord: event.currentTarget.value })}
    />
    <TextInput
      size="xs"
      label="Pack count"
      inputMode="numeric"
      description="How many individual units one scan of this barcode adds. Leave blank if you don't know yet."
      value={draft.packCount}
      onChange={(event) => onChange({ ...draft, packCount: event.currentTarget.value })}
    />
  </Stack>
);

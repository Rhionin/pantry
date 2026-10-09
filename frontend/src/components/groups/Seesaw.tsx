import { useState } from 'react';
import { Button, Group, NumberInput, Text } from '@mantine/core';
import type { GroupTarget, ProductGroup } from '../../types';
import { clampMonth, type SeesawDraft, type SeesawEnd, type SeesawPin, type SeesawView } from './seesaw';

function PinIcon() {
  return (
    <svg className="seesaw-pin" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
      <path
        fill="currentColor"
        d="M8 1.2a4.2 4.2 0 0 0-4.2 4.2c0 3.2 4.2 8.4 4.2 8.4s4.2-5.2 4.2-8.4A4.2 4.2 0 0 0 8 1.2Zm0 5.6a1.4 1.4 0 1 1 0-2.8 1.4 1.4 0 0 1 0 2.8Z"
      />
    </svg>
  );
}

function Pill({ end, onOpen }: { end: SeesawEnd; onOpen: (pin: SeesawPin) => void }) {
  return (
    <button
      type="button"
      className={end.pinned ? 'seesaw-pill is-pinned' : 'seesaw-pill is-inferred'}
      aria-pressed={end.pinned}
      aria-label={end.labelText}
      onClick={() => onOpen(end.pin)}
    >
      {end.pinned && <PinIcon />}
      <span>{end.label}</span>
      {end.isDefault && <span className="seesaw-default">default</span>}
    </button>
  );
}

export function Seesaw({
  view,
  group,
  draft,
  onDraft,
  onSave,
}: {
  view: SeesawView;
  group: Pick<ProductGroup, 'quantity' | 'windowMonths'>;
  draft: SeesawDraft | null;
  onDraft: (draft: SeesawDraft | null) => void;
  onSave: (target: GroupTarget) => Promise<void>;
}) {
  const [error, setError] = useState('');
  const amountUnit = view.dimension === 'volume' ? 'Fluid ounces' : 'Ounces';
  const canClear = group.quantity !== undefined || group.windowMonths !== undefined;

  const open = (pin: SeesawPin) => {
    if (draft?.pin === pin) return;
    const value = pin === 'amount'
      ? (view.amountValue !== null ? Math.round(view.amountValue * 10) / 10 : '')
      : (view.monthsValue !== null ? clampMonth(view.monthsValue) : '');
    setError('');
    onDraft({ pin, value });
  };

  const cancel = () => {
    setError('');
    onDraft(null);
  };

  const save = async () => {
    if (!draft) return;
    setError('');
    try {
      if (draft.pin === 'amount') {
        const value = typeof draft.value === 'number' ? draft.value : Number(draft.value);
        if (!Number.isFinite(value) || value <= 0 || value > 999) {
          setError('Enter an amount greater than zero and at most 999 ounces.');
          return;
        }
        await onSave({ quantity: value, dimension: view.dimension });
      } else {
        const value = typeof draft.value === 'number' ? draft.value : Number(draft.value);
        if (!Number.isInteger(value) || value < 1 || value > 12) {
          setError('Enter a whole number of months from 1 to 12.');
          return;
        }
        await onSave({ windowMonths: value });
      }
      onDraft(null);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to save the target.');
    }
  };

  const clear = async () => {
    setError('');
    try {
      await onSave({ clear: true });
      onDraft(null);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to save the target.');
    }
  };

  const editor = (pin: SeesawPin) => (
    <NumberInput
      className="seesaw-editor"
      aria-label={pin === 'amount' ? amountUnit : 'Months'}
      value={draft?.value ?? ''}
      onChange={(value) => onDraft({ pin, value })}
      min={pin === 'time' ? 1 : 0}
      max={pin === 'time' ? 12 : 999}
      allowDecimal={pin === 'amount'}
      decimalScale={pin === 'amount' ? 1 : 0}
      hideControls
      size="sm"
      onKeyDown={(event) => {
        if (event.key === 'Escape') {
          event.preventDefault();
          cancel();
        }
      }}
    />
  );

  const unknownAtEnd = !view.time.known;

  return (
    <form
      className="seesaw"
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
    >
      <p className="seesaw-caption">KEEP ON HAND</p>
      <div className="seesaw-row">
        <div className="seesaw-end">
          {draft?.pin === 'amount' ? editor('amount') : <Pill end={view.amount} onOpen={open} />}
        </div>
        <div className="seesaw-beam">
          <span className="seesaw-beam-line" />
          {view.rateLabel !== '' && <span className="seesaw-rate">{view.rateLabel}</span>}
        </div>
        <div className="seesaw-end">
          {draft?.pin === 'time' ? editor('time') : <Pill end={view.time} onOpen={open} />}
        </div>
      </div>
      {view.unknownHint !== '' && (
        <p className={unknownAtEnd ? 'seesaw-unknown is-end' : 'seesaw-unknown'}>{view.unknownHint}</p>
      )}
      {draft === null && view.unknownHint === '' && (
        <p className="seesaw-hint">Tap either end to set it; the other follows</p>
      )}
      {draft !== null && (
        <Group className="seesaw-actions" gap="sm">
          <Button type="submit" size="sm">Save</Button>
          <Button type="button" size="sm" variant="default" onClick={cancel}>Cancel</Button>
          {draft.pin === 'time' && canClear && (
            <Button type="button" size="sm" variant="subtle" onClick={() => void clear()}>
              Use the household default
            </Button>
          )}
        </Group>
      )}
      {error !== '' && <Text className="seesaw-error" size="sm" c="red">{error}</Text>}
    </form>
  );
}

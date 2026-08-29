import { useState } from 'react';
import { Alert, Button, Checkbox, Group, Modal, Radio, Stack } from '@mantine/core';
import { createProductOverride } from '../../api/client';
import type { ProductSummary } from '../../types';

export interface DisambiguationModalProps {
  opened: boolean;
  barcode: string;
  products: ProductSummary[];
  onClose: () => void;
  onSelect: (product: ProductSummary) => void;
}

export const DisambiguationModal = ({
  opened,
  barcode,
  products,
  onClose,
  onSelect,
}: DisambiguationModalProps) => {
  const [selectedId, setSelectedId] = useState('');
  const [saveOverride, setSaveOverride] = useState(false);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const handleConfirm = async () => {
    const selected = products.find((product) => product.id === selectedId);
    if (selected === undefined) return;
    setSubmitting(true);
    setError('');
    try {
      if (saveOverride) await createProductOverride({ barcode, productId: selected.id });
      onSelect(selected);
      onClose();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to save product choice.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal opened={opened} onClose={onClose} title="Choose a product">
      <Stack>
        <Radio.Group label="Products matching this barcode" value={selectedId} onChange={setSelectedId}>
          <Stack gap="xs" mt="xs">
            {products.map((product) => (
              <Radio key={product.id} value={product.id} label={`${product.name} — ${product.category}`} />
            ))}
          </Stack>
        </Radio.Group>
        <Checkbox
          label="Remember this choice for future scans"
          checked={saveOverride}
          onChange={(event) => setSaveOverride(event.currentTarget.checked)}
        />
        {error !== '' && <Alert color="red">{error}</Alert>}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>Cancel</Button>
          <Button
            disabled={selectedId === ''}
            loading={submitting}
            onClick={() => void handleConfirm()}
          >
            Use product
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
};

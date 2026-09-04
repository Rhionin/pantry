import { useState } from 'react';
import { Alert, Button, Group, NativeSelect, Stack, TextInput } from '@mantine/core';
import { batchCommitScanEntries } from '../../api/client';
import type { BatchCommitResponse, ScanDirection } from '../../types';
import { expiryDateToISOString } from './queueUtils';

export interface BatchReviewPanelProps {
  selectedIds: string[];
  onComplete: (response: BatchCommitResponse) => void;
}

export const BatchReviewPanel = ({ selectedIds, onComplete }: BatchReviewPanelProps) => {
  const [direction, setDirection] = useState<ScanDirection>('stock_in');
  const [expiryDate, setExpiryDate] = useState('');
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const handleConfirm = async () => {
    setSubmitting(true);
    setError('');
    try {
      const response = await batchCommitScanEntries({
        scanEntryIds: selectedIds,
        direction,
        expiresAt: direction === 'stock_in' ? expiryDateToISOString(expiryDate) : undefined,
        commit: true,
      });
      onComplete(response);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to commit selected scans.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Stack component="section" aria-labelledby="batch-review-heading" gap="xs">
      <h2 id="batch-review-heading">Batch review</h2>
      <Group align="end" gap="xs">
        <NativeSelect
          size="xs"
          label="Direction"
          value={direction}
          onChange={(event) => setDirection(event.currentTarget.value as ScanDirection)}
          data={[
            { value: 'stock_in', label: 'Stock in' },
            { value: 'stock_out', label: 'Stock out' },
          ]}
        />
        <TextInput
          size="xs"
          label="Expiration date"
          type="date"
          value={expiryDate}
          disabled={direction === 'stock_out'}
          onChange={(event) => setExpiryDate(event.currentTarget.value)}
        />
        <Button
          size="xs"
          onClick={() => void handleConfirm()}
          disabled={selectedIds.length === 0}
          loading={submitting}
        >
          Commit {selectedIds.length} selected
        </Button>
      </Group>
      {error !== '' && (
        <Alert color="red" py="xs">
          {error}
        </Alert>
      )}
    </Stack>
  );
};

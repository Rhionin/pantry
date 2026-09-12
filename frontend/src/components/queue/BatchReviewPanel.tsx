import { useState } from 'react';
import { Alert, Button, Group, Stack } from '@mantine/core';
import { batchCommitScanEntries } from '../../api/client';
import type { BatchCommitResponse } from '../../types';

export interface BatchReviewPanelProps {
  selectedIds: string[];
  onComplete: (response: BatchCommitResponse) => void;
}

export const BatchReviewPanel = ({ selectedIds, onComplete }: BatchReviewPanelProps) => {
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const handleApprove = async () => {
    setSubmitting(true);
    setError('');
    try {
      const response = await batchCommitScanEntries({
        scanEntryIds: selectedIds,
        commit: true,
      });
      onComplete(response);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to approve selected scans.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Stack component="section" aria-labelledby="approve-heading" gap="xs">
      <h2 id="approve-heading">Approve scans</h2>
      <Group align="end" gap="xs">
        <Button
          size="xs"
          onClick={() => void handleApprove()}
          disabled={selectedIds.length === 0}
          loading={submitting}
          aria-label={`Approve ${selectedIds.length} selected scans`}
        >
          Approve {selectedIds.length} selected
        </Button>
      </Group>
      {error !== '' && (
        <Alert color="red" py="xs">
          Unable to approve selected scans.
        </Alert>
      )}
    </Stack>
  );
};

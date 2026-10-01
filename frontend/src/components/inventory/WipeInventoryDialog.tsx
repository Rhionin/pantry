import { useState, type FormEvent } from 'react';
import { Alert, Button, Group, Modal, Stack, Text, TextInput } from '@mantine/core';
import { WIPE_INVENTORY_CONFIRMATION, wipeInventory } from '../../api/client';

export interface WipeInventoryDialogProps {
  onWiped: () => void;
}

export const WipeInventoryDialog = ({ onWiped }: WipeInventoryDialogProps) => {
  const [opened, setOpened] = useState(false);
  const [confirmation, setConfirmation] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  const close = () => {
    setConfirmation('');
    setError('');
    setOpened(false);
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (confirmation !== WIPE_INVENTORY_CONFIRMATION) {
      return;
    }

    setSubmitting(true);
    setError('');
    try {
      await wipeInventory(confirmation);
      close();
      onWiped();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to wipe inventory.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <Group justify="flex-end">
        <Button size="xs" variant="outline" color="red" onClick={() => setOpened(true)}>
          Wipe inventory
        </Button>
      </Group>
      <Modal opened={opened} onClose={close} title="Wipe inventory">
        <form onSubmit={(event) => void handleSubmit(event)}>
          <Stack gap="sm">
            <Text size="sm">
              This permanently removes every item in your inventory, including stocked
              units and shopping-list entries for those items. Saved products and barcode
              lookups are kept, so scanning still recognizes them.
            </Text>
            <Text size="sm">This cannot be undone.</Text>
            <TextInput
              label={`Type ${WIPE_INVENTORY_CONFIRMATION} to confirm`}
              autoComplete="off"
              value={confirmation}
              onChange={(event) => setConfirmation(event.currentTarget.value)}
            />
            {error !== '' && (
              <Alert color="red" py="xs">
                {error}
              </Alert>
            )}
            <Group justify="flex-end">
              <Button size="xs" variant="default" type="button" onClick={close}>Cancel</Button>
              <Button
                size="xs"
                color="red"
                type="submit"
                loading={submitting}
                disabled={confirmation !== WIPE_INVENTORY_CONFIRMATION}
              >
                Confirm wipe
              </Button>
            </Group>
          </Stack>
        </form>
      </Modal>
    </>
  );
};

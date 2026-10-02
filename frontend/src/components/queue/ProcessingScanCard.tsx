import { Card, Group, Loader, Stack, Text, Title } from '@mantine/core';
import type { ProcessingNotice } from '../../types';

export interface ProcessingScanCardProps {
  notice: ProcessingNotice;
  justCaptured?: boolean;
}

export const ProcessingScanCard = ({ notice, justCaptured = false }: ProcessingScanCardProps) => (
  <Card
    component="article"
    withBorder
    padding="sm"
    aria-busy="true"
    className={justCaptured ? 'processing-scan-card--just-captured' : undefined}
    aria-label={`Scan ${notice.barcode} processing`}
  >
    <Group gap="xs" wrap="nowrap" align="flex-start">
      <Loader size="sm" />
      <Stack gap={2}>
        <Title order={3} size="h5">Looking up product</Title>
        <Text size="xs" c="dimmed">Barcode: {notice.barcode}</Text>
        <Text size="xs">Checking the open food database…</Text>
      </Stack>
    </Group>
  </Card>
);

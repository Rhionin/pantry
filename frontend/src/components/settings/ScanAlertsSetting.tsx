import { useId, useState } from 'react';
import { Stack, Switch, Text } from '@mantine/core';
import {
  readScanAlertPermission,
  readScanAlertsEnabled,
  requestScanAlertPermissionNow,
  writeScanAlertsEnabled,
  type ScanAlertPermission,
} from '../queue/scanAlertPreference';

const timing = 'Failed scans and unrecognized products alert right away. A stock in or stock out batch alerts once, 5 minutes after the last scan. Alerts only arrive while this tab is open.';

export const ScanAlertsSetting = () => {
  const descriptionId = useId();
  const [enabled, setEnabled] = useState(readScanAlertsEnabled);
  const [permission, setPermission] = useState<ScanAlertPermission>(readScanAlertPermission);

  const onToggle = (next: boolean) => {
    writeScanAlertsEnabled(next);
    setEnabled(next);
    if (!next) return;
    const current = readScanAlertPermission();
    setPermission(current);
    if (current !== 'default') return;
    void requestScanAlertPermissionNow().then(setPermission);
  };

  return (
    <Stack gap={6} maw={420} data-scan-alerts-control>
      <Switch
        label="Scan alerts"
        aria-describedby={descriptionId}
        checked={enabled}
        color="green"
        onChange={(event) => {
          onToggle(event.currentTarget.checked);
        }}
      />
      <Text id={descriptionId} size="sm" c="dimmed">
        {timing}
      </Text>
      {enabled && permission === 'denied' && (
        <Text size="sm" c="red.8" role="status">
          Scan alerts are blocked. Re-allow notifications for this site in the browser's site settings.
        </Text>
      )}
      {enabled && permission === 'unsupported' && (
        <Text size="sm" c="dimmed" role="status">
          This browser cannot show notifications from an open tab.
        </Text>
      )}
    </Stack>
  );
};

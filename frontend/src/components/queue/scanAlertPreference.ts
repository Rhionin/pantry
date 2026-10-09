export const SCAN_ALERTS_ENABLED_KEY = 'pantry-scan-alerts';
export const SCAN_ALERTS_ASKED_KEY = 'pantry-scan-alerts-asked';
export const SCAN_ALERTS_CONTROL_SELECTOR = '[data-scan-alerts-control]';

export type ScanAlertPermission = NotificationPermission | 'unsupported';

export function readScanAlertsEnabled(): boolean {
  try {
    return localStorage.getItem(SCAN_ALERTS_ENABLED_KEY) !== 'off';
  } catch {
    return true;
  }
}

export function writeScanAlertsEnabled(enabled: boolean): void {
  localStorage.setItem(SCAN_ALERTS_ENABLED_KEY, enabled ? 'on' : 'off');
}

export function readScanAlertsAsked(): boolean {
  try {
    return localStorage.getItem(SCAN_ALERTS_ASKED_KEY) === '1';
  } catch {
    return false;
  }
}

export function markScanAlertsAsked(): void {
  localStorage.setItem(SCAN_ALERTS_ASKED_KEY, '1');
}

export function readScanAlertPermission(): ScanAlertPermission {
  if (typeof Notification === 'undefined') return 'unsupported';
  return Notification.permission;
}

// The settings switch is the explicit control. A click or key there turns
// alerts on or off; it must not also count as the automatic prompt.
export function gestureTargetsScanAlertsControl(event: Event): boolean {
  const target = event.target;
  const element = target instanceof Element
    ? target
    : target instanceof Node
      ? target.parentElement
      : null;
  return element?.closest(SCAN_ALERTS_CONTROL_SELECTOR) != null;
}

const canAutoAsk = (): boolean =>
  readScanAlertsEnabled()
  && !readScanAlertsAsked()
  && readScanAlertPermission() === 'default';

// Chrome shows the permission prompt from a user gesture and quiets repeats.
// Remember the ask even when the prompt is dismissed or throws, so this
// browser is asked once.
export function requestScanAlertPermissionOnce(): boolean {
  if (!canAutoAsk()) return false;
  markScanAlertsAsked();
  void Promise.resolve(Notification.requestPermission()).catch(() => {});
  return true;
}

// Turning the switch on is its own gesture, including after the automatic
// ask was dismissed and permission is still undecided.
export async function requestScanAlertPermissionNow(): Promise<ScanAlertPermission> {
  if (readScanAlertPermission() !== 'default') return readScanAlertPermission();
  markScanAlertsAsked();
  try {
    return await Promise.resolve(Notification.requestPermission());
  } catch {
    return readScanAlertPermission();
  }
}

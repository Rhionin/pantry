import type { PushSubscribeFailure } from '../queue/pushAlerts';

export type AlertPermission = NotificationPermission | 'unsupported';
export type ScanAlertPhase = 'idle' | 'checking' | 'turning-on' | 'turning-off';
type StatusTone = 'quiet' | 'off' | 'on' | 'problem';

export const scanAlertScopeCopy = 'This browser only. Another browser, or another device, has its own switch.';

const failureCopy: Record<PushSubscribeFailure, string> = {
  'no-push': 'This browser has no push service. Alerts arrive only while this tab is open.',
  'register-failed': 'The browser could not register the service worker. Alerts arrive only while this tab is open.',
  'empty-vapid': 'This pantry has no push key. Alerts arrive only while this tab is open.',
  'subscribe-rejected': 'The browser refused the push subscription. Alerts arrive only while this tab is open.',
  'save-failed': 'Pantry could not save this device. Alerts arrive only while this tab is open.',
};

export function scanAlertStatus(
  permission: AlertPermission,
  preference: boolean,
  phase: ScanAlertPhase,
  failure: PushSubscribeFailure | null,
): { text: string; tone: StatusTone; retry: boolean } {
  if (permission === 'unsupported') {
    return { text: 'This browser cannot show notifications.', tone: 'quiet', retry: false };
  }
  if (permission === 'denied') {
    return { text: 'Blocked in browser settings', tone: 'problem', retry: false };
  }
  if (phase === 'checking') {
    return { text: 'Checking this device', tone: 'quiet', retry: false };
  }
  if (phase === 'turning-on') {
    return { text: 'Turning scan alerts on', tone: 'quiet', retry: false };
  }
  if (phase === 'turning-off') {
    return { text: 'Turning scan alerts off', tone: 'quiet', retry: false };
  }
  if (!preference) {
    if (failure === 'save-failed') {
      return {
        text: 'Pantry could not remove this device. It may still send alerts.',
        tone: 'problem',
        retry: true,
      };
    }
    return { text: 'Off', tone: 'off', retry: false };
  }
  if (failure !== null) {
    return { text: failureCopy[failure], tone: 'problem', retry: true };
  }
  return { text: 'On: alerts arrive even when Pantry is closed', tone: 'on', retry: false };
}

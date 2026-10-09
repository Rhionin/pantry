import { useState } from 'react';

type AlertPermission = NotificationPermission | 'unsupported';

const readNotificationPermission = (): AlertPermission => {
  if (typeof Notification === 'undefined') return 'unsupported';
  return Notification.permission;
};

const explainer = 'Failed scans and unrecognized products alert right away. A stock in or stock out batch alerts once, 5 minutes after the last scan. Alerts only arrive while this tab is open.';

// Permission is requested only from this button. Loading the scan queue does not prompt.
export function ScanAlertPrompt() {
  const [permission, setPermission] = useState(readNotificationPermission);
  const [asking, setAsking] = useState(false);

  if (permission === 'granted') return null;

  if (permission === 'unsupported') {
    return (
      <div className="scan-alert-prompt">
        <p>This browser cannot show notifications from an open tab.</p>
      </div>
    );
  }

  if (permission === 'denied') {
    return (
      <div className="scan-alert-prompt">
        <p>Scan alerts are blocked. Allow notifications for this site in the browser settings.</p>
      </div>
    );
  }

  const ask = () => {
    setAsking(true);
    void Promise.resolve(Notification.requestPermission())
      .then((result) => {
        setPermission(result);
      })
      .catch(() => {
        setPermission(readNotificationPermission());
      })
      .finally(() => {
        setAsking(false);
      });
  };

  return (
    <div className="scan-alert-prompt">
      <button
        type="button"
        className="scan-alert-enable"
        aria-describedby="scan-alert-explainer"
        disabled={asking}
        onClick={ask}
      >
        {asking ? 'Waiting for a choice' : 'Notify me about scans'}
      </button>
      <p id="scan-alert-explainer">{explainer}</p>
    </div>
  );
}

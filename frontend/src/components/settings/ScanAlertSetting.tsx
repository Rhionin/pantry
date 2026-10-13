import { useEffect, useId, useRef, useState } from 'react';
import { Input } from '@mantine/core';
import {
  disableScanAlerts,
  ensurePushSubscription,
  isPushSubscribeFailure,
  scanAlertsPreferred,
  writeScanAlertPreference,
  type PushSubscribeFailure,
  type PushSubscribeResult,
} from '../queue/pushAlerts';
import { scanAlertScopeCopy, scanAlertStatus, type AlertPermission, type ScanAlertPhase } from './scanAlertCopy';

const readPermission = (): AlertPermission => {
  if (typeof Notification === 'undefined') return 'unsupported';
  return Notification.permission;
};

export function ScanAlertSetting() {
  const switchId = useId();
  const [preference, setPreference] = useState(scanAlertsPreferred);
  const [permission, setPermission] = useState(readPermission);
  const [failure, setFailure] = useState<PushSubscribeFailure | null>(null);
  const [phase, setPhase] = useState<ScanAlertPhase>(() => (
    scanAlertsPreferred() && readPermission() === 'granted' ? 'checking' : 'idle'
  ));
  const request = useRef(0);

  function applyResult(result: PushSubscribeResult) {
    if (result.ok) {
      setFailure(null);
      setPreference(true);
      return;
    }
    if (result.reason === 'denied') {
      setPermission('denied');
      setPreference(false);
      setFailure(null);
      return;
    }
    if (result.reason === 'unavailable') {
      setPermission('unsupported');
      setPreference(false);
      setFailure(null);
      return;
    }
    if (isPushSubscribeFailure(result.reason)) {
      setFailure(result.reason);
      return;
    }
    setFailure(null);
  }

  useEffect(() => {
    if (!scanAlertsPreferred() || readPermission() !== 'granted') return undefined;
    const id = ++request.current;
    void ensurePushSubscription().then((result) => {
      if (request.current !== id) return;
      applyResult(result);
      setPhase('idle');
    });
    return undefined;
  }, []);

  const turnOn = async () => {
    const id = ++request.current;
    setFailure(null);
    setPhase('turning-on');
    setPreference(true);
    let nextPermission = readPermission();
    if (nextPermission === 'default') {
      try {
        nextPermission = await Notification.requestPermission();
      } catch {
        nextPermission = readPermission();
      }
    }
    if (request.current !== id) return;
    setPermission(nextPermission);
    if (nextPermission !== 'granted') {
      writeScanAlertPreference(false);
      setPreference(false);
      setPhase('idle');
      return;
    }
    writeScanAlertPreference(true);
    const result = await ensurePushSubscription();
    if (request.current !== id) return;
    applyResult(result);
    setPhase('idle');
  };

  const turnOff = async () => {
    const id = ++request.current;
    setPreference(false);
    setFailure(null);
    setPhase('turning-off');
    const problem = await disableScanAlerts();
    if (request.current !== id) return;
    setFailure(problem);
    setPhase('idle');
  };

  const retry = () => {
    if (scanAlertsPreferred()) void turnOn();
    else void turnOff();
  };

  const onToggle = (next: boolean) => {
    if (permission === 'unsupported' || permission === 'denied') return;
    if (next) void turnOn();
    else void turnOff();
  };

  const status = scanAlertStatus(permission, preference, phase, failure);
  const blocked = permission === 'unsupported' || permission === 'denied';

  return (
    <div className="scan-alert-setting">
      <div className="scan-alert-head">
        <Input.Label htmlFor={switchId} className="scan-alert-label" data-disabled={blocked || undefined}>
          Scan alerts on this device
        </Input.Label>
        <span className="scan-alert-switch">
          <input
            id={switchId}
            type="checkbox"
            role="switch"
            checked={preference && permission !== 'unsupported'}
            disabled={blocked}
            aria-describedby="scan-alert-scope"
            onChange={(event) => onToggle(event.currentTarget.checked)}
          />
          <span className="scan-alert-switch-track" aria-hidden="true" />
        </span>
        <Input.Description id="scan-alert-scope">
          {scanAlertScopeCopy}
        </Input.Description>
      </div>
      <p id="scan-alert-status" role="status" className={`scan-alert-status scan-alert-status--${status.tone}`}>
        {status.text}
      </p>
      {status.retry && (
        <button type="button" className="scan-alert-retry" aria-describedby="scan-alert-status" onClick={retry}>
          Retry
        </button>
      )}
    </div>
  );
}

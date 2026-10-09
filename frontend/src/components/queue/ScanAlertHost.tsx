import { useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import type { ProcessingFailure, ScanEntry } from '../../types';
import { openScanAlert, showScanAlert } from './browserScanAlert';
import { gestureTargetsScanAlertsControl, requestScanAlertPermissionOnce } from './scanAlertPreference';
import { ScanAttention, type AttentionNotice } from './scanAttention';

const parseEvent = <T,>(message: Event): T | null => {
  const data = (message as MessageEvent).data;
  if (typeof data !== 'string') return null;
  try {
    return JSON.parse(data) as T;
  } catch {
    return null;
  }
};

// Listens for the same scan events the queue renders, on every signed-in
// page, so an alert can fire while the tab is open on Inventory or elsewhere.
// The stream is not the telemetry stream: a second gap tracker would see
// interleaving ids and report false misses.
export function ScanAlertHost() {
  const navigate = useNavigate();
  const navigateRef = useRef(navigate);
  useEffect(() => {
    navigateRef.current = navigate;
  }, [navigate]);

  useEffect(() => {
    // Chrome only presents the permission prompt from a user gesture, and it
    // quiets a prompt that is asked again. The first pointer, key, or scan
    // in this browser is the ask, unless scan alerts are switched off.
    const onGesture = (event: Event) => {
      if (gestureTargetsScanAlertsControl(event)) return;
      requestScanAlertPermissionOnce();
    };
    window.addEventListener('pointerdown', onGesture, true);
    window.addEventListener('keydown', onGesture, true);

    const open = (notice: AttentionNotice) => {
      window.focus();
      openScanAlert(notice.direction);
      if (window.location.pathname !== '/') navigateRef.current('/');
    };
    const attention = new ScanAttention((notice) => {
      showScanAlert(notice, () => open(notice));
    });
    const source = new EventSource('/api/events');
    source.addEventListener('scan', (message) => {
      requestScanAlertPermissionOnce();
      const entry = parseEvent<ScanEntry>(message);
      if (entry !== null) attention.noteScan(entry);
    });
    source.addEventListener('scan_processing_failed', (message) => {
      requestScanAlertPermissionOnce();
      const failure = parseEvent<ProcessingFailure>(message);
      if (failure !== null) attention.noteFailure(failure);
    });
    return () => {
      window.removeEventListener('pointerdown', onGesture, true);
      window.removeEventListener('keydown', onGesture, true);
      attention.dispose();
      source.close();
    };
  }, []);

  return null;
}

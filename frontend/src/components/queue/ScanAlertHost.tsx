import { useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { notifications, notificationsStore } from '@mantine/notifications';
import type { ProcessingFailure, ScanEntry } from '../../types';
import { openScanAlert, showScanAlert } from './browserScanAlert';
import { directionFromAlertURL, ensurePushSubscription, releasePushSubscription, scanAlertsPreferred, setBackgroundAlerts } from './pushAlerts';
import { ScanAttention, type AttentionNotice } from './scanAttention';

interface ForegroundAlert {
  title: string;
  body: string;
  tag: string;
}

// The service worker posts this when a Stockbot window is already visible,
// in place of a system notification. The toast stays until the person opens
// the queue or dismisses it, matching requireInteraction on the system alert.
function showForegroundAlert(alert: ForegroundAlert, onOpen: () => void): void {
  const title = alert.title.trim() !== '' ? alert.title : 'Stockbot';
  const body = alert.body.trim();
  const tag = alert.tag.trim();
  // The system notification uses the same tag to replace itself. show()
  // ignores an id that is already on screen, so a repeat has to update.
  let shownId = tag;
  const message = (
    <div className="scan-alert-toast">
      {body !== '' ? <p className="scan-alert-toast-body">{body}</p> : null}
      <button
        type="button"
        className="scan-alert-toast-open"
        onClick={() => {
          if (shownId !== '') notifications.hide(shownId);
          onOpen();
        }}
      >
        Open the scan queue
      </button>
    </div>
  );
  const data = {
    title,
    message,
    color: 'amber' as const,
    autoClose: false as const,
    position: 'top-center' as const,
  };
  if (tag !== '' && notificationsStore.getState().notifications.some((item) => item.id === tag)) {
    notifications.update({ ...data, id: tag });
    return;
  }
  shownId = notifications.show(tag !== '' ? { ...data, id: tag } : data);
}

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
    if (typeof Notification !== 'undefined' && Notification.permission === 'denied') {
      setBackgroundAlerts(false);
      void releasePushSubscription();
      return;
    }
    if (!scanAlertsPreferred()) {
      setBackgroundAlerts(false);
      void releasePushSubscription();
      return;
    }
    void ensurePushSubscription();
  }, []);

  useEffect(() => {
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
      const entry = parseEvent<ScanEntry>(message);
      if (entry !== null) attention.noteScan(entry);
    });
    source.addEventListener('scan_processing_failed', (message) => {
      const failure = parseEvent<ProcessingFailure>(message);
      if (failure !== null) attention.noteFailure(failure);
    });
    const onMessage = (event: MessageEvent) => {
      const data = event.data as { type?: string; url?: string; title?: string; body?: string; tag?: string } | null;
      if (data == null || typeof data !== 'object') return;
      if (data.type === 'pantry-scan-alert-toast') {
        if (!scanAlertsPreferred()) return;
        const url = typeof data.url === 'string' ? data.url : '';
        const title = typeof data.title === 'string' ? data.title : '';
        const body = typeof data.body === 'string' ? data.body : '';
        const tag = typeof data.tag === 'string' ? data.tag : '';
        showForegroundAlert({ title, body, tag }, () => {
          open({
            title,
            body,
            tag,
            direction: directionFromAlertURL(url),
          });
        });
        return;
      }
      if (data.type !== 'pantry-scan-alert') return;
      open({
        title: '',
        body: '',
        tag: '',
        direction: directionFromAlertURL(typeof data.url === 'string' ? data.url : ''),
      });
    };
    if ('serviceWorker' in navigator) {
      navigator.serviceWorker.addEventListener('message', onMessage);
    }
    return () => {
      attention.dispose();
      source.close();
      if ('serviceWorker' in navigator) {
        navigator.serviceWorker.removeEventListener('message', onMessage);
      }
    };
  }, []);

  return null;
}

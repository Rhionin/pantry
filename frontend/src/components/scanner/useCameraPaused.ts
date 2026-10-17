import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { nextCameraSession, type CameraSession, type CameraSessionSignal } from './cameraSession';

// True while an opted-in scan has released the camera because the page is
// hidden. Becoming visible resumes only a scan that was still running.
// Stopping the scan clears the pause, so a later visibility event cannot
// open the camera again. Listeners are removed on unmount.
export function useCameraPaused(scanning: boolean, onPause: () => void): boolean {
  const [suspended, setSuspended] = useState(false);
  const [trackedScanning, setTrackedScanning] = useState(scanning);

  let suspendedNow = suspended;
  if (scanning !== trackedScanning) {
    setTrackedScanning(scanning);
    const previous: CameraSession = !trackedScanning ? 'stopped' : suspended ? 'paused' : 'running';
    const next = nextCameraSession(previous, scanning ? { type: 'start' } : { type: 'stop' });
    suspendedNow = next === 'paused';
    if (suspendedNow !== suspended) setSuspended(suspendedNow);
  }

  const onPauseRef = useRef(onPause);
  const scanningRef = useRef(scanning);
  const suspendedRef = useRef(suspendedNow);

  useEffect(() => {
    onPauseRef.current = onPause;
  }, [onPause]);

  // Layout, not passive: a visibility event can arrive after paint and must
  // see the session committed for this render, not a ref write that is still
  // queued.
  useLayoutEffect(() => {
    scanningRef.current = scanning;
    suspendedRef.current = suspendedNow;
  }, [scanning, suspendedNow]);

  useEffect(() => {
    function session(): CameraSession {
      if (!scanningRef.current) return 'stopped';
      return suspendedRef.current ? 'paused' : 'running';
    }

    function commit(signal: CameraSessionSignal) {
      const previous = session();
      const next = nextCameraSession(previous, signal);
      if (next === previous) return;
      const release = previous === 'running' && next === 'paused';
      suspendedRef.current = next === 'paused';
      setSuspended(next === 'paused');
      if (release) onPauseRef.current();
    }

    const onVisibility = () => {
      commit({ type: 'visibilitychange', visibilityState: document.visibilityState });
    };
    const onPageHide = () => {
      commit({ type: 'pagehide' });
    };

    document.addEventListener('visibilitychange', onVisibility);
    window.addEventListener('pagehide', onPageHide);
    return () => {
      document.removeEventListener('visibilitychange', onVisibility);
      window.removeEventListener('pagehide', onPageHide);
    };
  }, []);

  return scanning && suspendedNow;
}

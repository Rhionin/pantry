// The scan loop skips decoding while the document is hidden, but that leaves
// the MediaStream open and the camera indicator lit. This session is the
// decision for releasing those tracks, and for bringing them back only when
// the user had not stopped scanning.

export type CameraSession = 'running' | 'paused' | 'stopped';

export type CameraSessionSignal =
  | { type: 'start' }
  | { type: 'stop' }
  | { type: 'pagehide' }
  | { type: 'visibilitychange'; visibilityState: DocumentVisibilityState };

export function nextCameraSession(session: CameraSession, signal: CameraSessionSignal): CameraSession {
  switch (signal.type) {
    case 'start':
      return 'running';
    case 'stop':
      return 'stopped';
    case 'pagehide':
      return session === 'running' ? 'paused' : session;
    case 'visibilitychange':
      if (signal.visibilityState === 'hidden' && session === 'running') return 'paused';
      if (signal.visibilityState === 'visible' && session === 'paused') return 'running';
      return session;
  }
}

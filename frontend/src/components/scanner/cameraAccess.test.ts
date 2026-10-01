import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  cameraBlockReason,
  describeCameraError,
  openCameraStream,
  shouldAcceptScan,
} from './cameraAccess';

describe('cameraBlockReason', () => {
  const originalSecure = window.isSecureContext;

  afterEach(() => {
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: originalSecure });
    vi.unstubAllGlobals();
  });

  it('explains that camera scanning needs HTTPS when the page is not a secure context', () => {
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: false });

    expect(cameraBlockReason()).toEqual({
      kind: 'insecure',
      message: expect.stringMatching(/https/i),
    });
  });

  it('explains that this browser has no camera API', () => {
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true });
    vi.stubGlobal('navigator', { ...navigator, mediaDevices: undefined });

    expect(cameraBlockReason()?.kind).toBe('unavailable');
  });

  it('allows a secure page that can request a camera', () => {
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true });
    vi.stubGlobal('navigator', {
      ...navigator,
      mediaDevices: { getUserMedia: vi.fn() },
    });

    expect(cameraBlockReason()).toBeNull();
  });
});

describe('describeCameraError', () => {
  it('names permission, missing camera, busy camera, insecure, and timeout failures', () => {
    expect(describeCameraError(new DOMException('no', 'NotAllowedError'))).toMatch(/camera access was denied/i);
    expect(describeCameraError(new DOMException('no', 'NotFoundError'))).toMatch(/no camera was found/i);
    expect(describeCameraError(new DOMException('no', 'NotReadableError'))).toMatch(/already in use/i);
    expect(describeCameraError(new DOMException('no', 'SecurityError'))).toMatch(/https/i);
    expect(describeCameraError(new DOMException('no', 'AbortError'))).toMatch(/did not respond/i);
  });

  it('uses a generic message for an unexpected failure', () => {
    expect(describeCameraError(new Error('boom'))).toMatch(/denied or is unavailable/i);
  });
});

describe('shouldAcceptScan', () => {
  it('accepts the first read, a different code, and the same code after the cooldown', () => {
    expect(shouldAcceptScan(null, '111', 1_000, 2_000)).toBe(true);
    expect(shouldAcceptScan({ value: '111', at: 1_000 }, '222', 1_100, 2_000)).toBe(true);
    expect(shouldAcceptScan({ value: '111', at: 1_000 }, '111', 2_000, 2_000)).toBe(false);
    expect(shouldAcceptScan({ value: '111', at: 1_000 }, '111', 3_001, 2_000)).toBe(true);
  });
});

describe('openCameraStream', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('asks for the chosen facing camera and no microphone', async () => {
    const getUserMedia = vi.fn().mockResolvedValue({} as MediaStream);
    vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } });

    await openCameraStream('environment');

    expect(getUserMedia).toHaveBeenCalledWith({
      video: { facingMode: { ideal: 'environment' } },
      audio: false,
    });
  });

  it('retries with any camera when the facing-mode hint cannot be met', async () => {
    const stream = {} as MediaStream;
    const getUserMedia = vi.fn()
      .mockRejectedValueOnce(new DOMException('over', 'OverconstrainedError'))
      .mockResolvedValueOnce(stream);
    vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } });

    await expect(openCameraStream('environment')).resolves.toBe(stream);
    expect(getUserMedia).toHaveBeenLastCalledWith({ video: true, audio: false });
  });

  it('does not retry after the user denies permission', async () => {
    const getUserMedia = vi.fn().mockRejectedValue(new DOMException('no', 'NotAllowedError'));
    vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } });

    await expect(openCameraStream('user')).rejects.toMatchObject({ name: 'NotAllowedError' });
    expect(getUserMedia).toHaveBeenCalledTimes(1);
  });
});

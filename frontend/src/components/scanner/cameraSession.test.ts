import { describe, expect, it } from 'vitest';
import { nextCameraSession, type CameraSession, type CameraSessionSignal } from './cameraSession';

function step(session: CameraSession, signal: CameraSessionSignal): CameraSession {
  return nextCameraSession(session, signal);
}

describe('nextCameraSession', () => {
  it('pauses a running camera when the page is hidden or unloading', () => {
    expect(step('running', { type: 'visibilitychange', visibilityState: 'hidden' })).toBe('paused');
    expect(step('running', { type: 'pagehide' })).toBe('paused');
  });

  it('resumes a paused camera when the page is visible again', () => {
    expect(step('paused', { type: 'visibilitychange', visibilityState: 'visible' })).toBe('running');
  });

  it('does not resume a camera the user had already stopped', () => {
    expect(step('stopped', { type: 'visibilitychange', visibilityState: 'hidden' })).toBe('stopped');
    expect(step('stopped', { type: 'pagehide' })).toBe('stopped');
    expect(step('stopped', { type: 'visibilitychange', visibilityState: 'visible' })).toBe('stopped');
  });

  it('forgets a paused camera when the user stops, so a later show stays stopped', () => {
    const afterStop = step('paused', { type: 'stop' });
    expect(afterStop).toBe('stopped');
    expect(step(afterStop, { type: 'visibilitychange', visibilityState: 'visible' })).toBe('stopped');
    expect(step(afterStop, { type: 'pagehide' })).toBe('stopped');
  });

  it('keeps a paused camera paused across another hide, and a running camera running across a show', () => {
    expect(step('paused', { type: 'pagehide' })).toBe('paused');
    expect(step('paused', { type: 'visibilitychange', visibilityState: 'hidden' })).toBe('paused');
    expect(step('running', { type: 'visibilitychange', visibilityState: 'visible' })).toBe('running');
  });

  it('starts a stopped or paused camera only when the user asks', () => {
    expect(step('stopped', { type: 'start' })).toBe('running');
    expect(step('paused', { type: 'start' })).toBe('running');
    expect(step('running', { type: 'stop' })).toBe('stopped');
  });
});

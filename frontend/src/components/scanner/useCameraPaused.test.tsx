import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useCameraPaused } from './useCameraPaused';

function setPageVisibility(state: DocumentVisibilityState) {
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => state });
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => state === 'hidden' });
}

function hidePage() {
  setPageVisibility('hidden');
  document.dispatchEvent(new Event('visibilitychange'));
}

function showPage() {
  setPageVisibility('visible');
  document.dispatchEvent(new Event('visibilitychange'));
}

describe('useCameraPaused', () => {
  afterEach(() => {
    setPageVisibility('visible');
    vi.restoreAllMocks();
  });

  it('releases the camera when the page hides and resumes it when visible again', () => {
    const onPause = vi.fn();
    const { result } = renderHook(() => useCameraPaused(true, onPause));

    expect(result.current).toBe(false);

    act(() => hidePage());

    expect(onPause).toHaveBeenCalledTimes(1);
    expect(result.current).toBe(true);

    act(() => showPage());

    expect(onPause).toHaveBeenCalledTimes(1);
    expect(result.current).toBe(false);
  });

  it('releases the camera on pagehide', () => {
    const onPause = vi.fn();
    const { result } = renderHook(() => useCameraPaused(true, onPause));

    act(() => {
      window.dispatchEvent(new Event('pagehide'));
    });

    expect(onPause).toHaveBeenCalledTimes(1);
    expect(result.current).toBe(true);
  });

  it('does not release or resume a scan the user never started', () => {
    const onPause = vi.fn();
    const { result } = renderHook(() => useCameraPaused(false, onPause));

    act(() => {
      hidePage();
      window.dispatchEvent(new Event('pagehide'));
      showPage();
    });

    expect(onPause).not.toHaveBeenCalled();
    expect(result.current).toBe(false);
  });

  it('does not resume after the user stops scanning while the page is hidden', () => {
    const onPause = vi.fn();
    const { result, rerender } = renderHook(
      ({ scanning }) => useCameraPaused(scanning, onPause),
      { initialProps: { scanning: true } },
    );

    act(() => hidePage());
    expect(result.current).toBe(true);

    rerender({ scanning: false });
    expect(result.current).toBe(false);

    act(() => showPage());

    expect(onPause).toHaveBeenCalledTimes(1);
    expect(result.current).toBe(false);

    rerender({ scanning: true });
    expect(result.current).toBe(false);
  });

  it('does not restart a scan that was stopped before the page hid', () => {
    const onPause = vi.fn();
    const { result, rerender } = renderHook(
      ({ scanning }) => useCameraPaused(scanning, onPause),
      { initialProps: { scanning: true } },
    );

    rerender({ scanning: false });
    act(() => {
      hidePage();
      showPage();
    });

    expect(onPause).not.toHaveBeenCalled();
    expect(result.current).toBe(false);
  });

  it('removes the page listeners on unmount', () => {
    const onPause = vi.fn();
    const { unmount } = renderHook(() => useCameraPaused(true, onPause));

    unmount();
    act(() => {
      hidePage();
      window.dispatchEvent(new Event('pagehide'));
      showPage();
    });

    expect(onPause).not.toHaveBeenCalled();
  });
});

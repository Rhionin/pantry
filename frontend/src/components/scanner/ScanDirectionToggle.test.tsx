import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { useRef } from 'react';
import { ScanDirectionToggle, type ScanDirectionToggleHandle } from './ScanDirectionToggle';

const FIVE_MINUTES_MS = 5 * 60 * 1000;

// ScanDirectionToggle exposes recordScan via an imperative ref handle rather
// than a prop, since the parent scanning page calls it on each new scan
// rather than re-rendering the toggle. This harness gives tests a button to
// trigger that same call.
function ToggleHarness({ onDirectionChange }: { onDirectionChange: (direction: string | null) => void }) {
  const ref = useRef<ScanDirectionToggleHandle>(null);
  return (
    <>
      <ScanDirectionToggle ref={ref} onDirectionChange={onDirectionChange} />
      <button onClick={() => ref.current?.recordScan()}>record scan</button>
    </>
  );
}

function renderToggle(onDirectionChange: (direction: string | null) => void) {
  return render(
    <MantineProvider>
      <ToggleHarness onDirectionChange={onDirectionChange} />
    </MantineProvider>,
  );
}

function selectStockIn() {
  fireEvent.click(screen.getByRole('radio', { name: 'Stock In' }));
}

function recordScan() {
  fireEvent.click(screen.getByRole('button', { name: /record scan/i }));
}

describe('ScanDirectionToggle auto-clear', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('clears the pre-selected direction after 5 minutes of scanner idle time', () => {
    const onDirectionChange = vi.fn();
    renderToggle(onDirectionChange);

    selectStockIn();
    expect(onDirectionChange).toHaveBeenLastCalledWith('stock_in');

    act(() => {
      vi.advanceTimersByTime(FIVE_MINUTES_MS);
    });

    expect(onDirectionChange).toHaveBeenLastCalledWith(null);
    expect(screen.getByRole('radio', { name: 'Stock In' })).not.toBeChecked();
  });

  it('preserves the direction while under 5 minutes since selection', () => {
    const onDirectionChange = vi.fn();
    renderToggle(onDirectionChange);

    selectStockIn();
    act(() => {
      vi.advanceTimersByTime(FIVE_MINUTES_MS - 1);
    });

    expect(screen.getByRole('radio', { name: 'Stock In' })).toBeChecked();
    expect(onDirectionChange).not.toHaveBeenLastCalledWith(null);
  });

  it('preserves the direction when a scan resets the idle timer before 5 minutes elapse', () => {
    const onDirectionChange = vi.fn();
    renderToggle(onDirectionChange);

    selectStockIn();

    // A scan just before the original 5-minute deadline resets the timer.
    act(() => {
      vi.advanceTimersByTime(FIVE_MINUTES_MS - 1000);
    });
    recordScan();

    // Total elapsed time since selection now exceeds 5 minutes, but less
    // than 5 minutes have passed since the scan that reset the timer.
    act(() => {
      vi.advanceTimersByTime(FIVE_MINUTES_MS - 1000);
    });

    expect(screen.getByRole('radio', { name: 'Stock In' })).toBeChecked();
    expect(onDirectionChange).not.toHaveBeenLastCalledWith(null);
  });

  it('clears the direction once 5 minutes elapse since the last recorded scan', () => {
    const onDirectionChange = vi.fn();
    renderToggle(onDirectionChange);

    selectStockIn();
    act(() => {
      vi.advanceTimersByTime(FIVE_MINUTES_MS - 1000);
    });
    recordScan();

    act(() => {
      vi.advanceTimersByTime(FIVE_MINUTES_MS);
    });

    expect(onDirectionChange).toHaveBeenLastCalledWith(null);
    expect(screen.getByRole('radio', { name: 'Stock In' })).not.toBeChecked();
  });
});

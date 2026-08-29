import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { CameraScanner, type CameraScannerProps } from './CameraScanner';

function renderScanner(props: CameraScannerProps) {
  return render(
    <MantineProvider>
      <CameraScanner {...props} />
    </MantineProvider>,
  );
}

describe('CameraScanner', () => {
  const originalBarcodeDetector = window.BarcodeDetector;

  afterEach(() => {
    window.BarcodeDetector = originalBarcodeDetector;
    vi.restoreAllMocks();
  });

  describe('when BarcodeDetector is unavailable', () => {
    beforeEach(() => {
      // jsdom does not implement BarcodeDetector; explicitly clear it to
      // exercise the fallback path regardless of test environment.
      delete (window as { BarcodeDetector?: unknown }).BarcodeDetector;
    });

    it('renders a fallback message and manual entry option instead of the camera feed', () => {
      renderScanner({ onScan: vi.fn() });

      expect(screen.getByText(/camera scanning unavailable/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/barcode/i)).toBeInTheDocument();
      expect(screen.queryByRole('video' as never)).not.toBeInTheDocument();
    });

    it('calls onScan with the entered value when the manual entry form is submitted', () => {
      const onScan = vi.fn();
      renderScanner({ onScan });

      fireEvent.change(screen.getByLabelText(/barcode/i), { target: { value: '0123456789012' } });
      fireEvent.click(screen.getByRole('button', { name: /submit/i }));

      expect(onScan).toHaveBeenCalledWith('0123456789012');
    });

    it('does not call onScan when the manual entry is submitted blank', () => {
      const onScan = vi.fn();
      renderScanner({ onScan });

      fireEvent.click(screen.getByRole('button', { name: /submit/i }));

      expect(onScan).not.toHaveBeenCalled();
    });
  });

  describe('when camera permission is denied', () => {
    beforeEach(() => {
      window.BarcodeDetector = vi.fn(function BarcodeDetector() {
        return { detect: vi.fn().mockResolvedValue([]) };
      }) as unknown as typeof window.BarcodeDetector;
      vi.stubGlobal('navigator', {
        ...navigator,
        mediaDevices: {
          getUserMedia: vi.fn().mockRejectedValue(new Error('permission denied')),
        },
      });
    });

    it('falls back to manual entry rather than crashing', async () => {
      renderScanner({ onScan: vi.fn() });

      expect(await screen.findByText(/camera access was denied/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/barcode/i)).toBeInTheDocument();
    });
  });

  describe('when BarcodeDetector is available and camera access succeeds', () => {
    let detectMock: ReturnType<typeof vi.fn>;

    beforeEach(() => {
      detectMock = vi.fn().mockResolvedValue([]);
      window.BarcodeDetector = vi.fn(function BarcodeDetector() {
        return { detect: detectMock };
      }) as unknown as typeof window.BarcodeDetector;

      const fakeStream = { getTracks: () => [{ stop: vi.fn() }] } as unknown as MediaStream;
      vi.stubGlobal('navigator', {
        ...navigator,
        mediaDevices: {
          getUserMedia: vi.fn().mockResolvedValue(fakeStream),
        },
      });
      HTMLMediaElement.prototype.play = vi.fn().mockResolvedValue(undefined);
    });

    it('renders the live video feed with a scan overlay instead of the fallback UI', () => {
      const { container } = renderScanner({ onScan: vi.fn() });

      expect(container.querySelector('video')).not.toBeNull();
      expect(screen.queryByText(/camera scanning unavailable/i)).not.toBeInTheDocument();
    });

    it('requests camera access with the environment-facing camera', () => {
      renderScanner({ onScan: vi.fn() });

      expect(navigator.mediaDevices.getUserMedia).toHaveBeenCalledWith({
        video: { facingMode: 'environment' },
      });
    });
  });
});

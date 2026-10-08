import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { CameraScanner, type CameraScannerProps } from './CameraScanner';
import { RESCAN_COOLDOWN_MS } from './cameraAccess';
import type { DecodedBarcode, FrameDecoder } from './frameDecoder';

function renderScanner(props: CameraScannerProps) {
  return render(
    <MantineProvider>
      <CameraScanner {...props} />
    </MantineProvider>,
  );
}

function installSecureCamera(getUserMedia: ReturnType<typeof vi.fn>) {
  Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true });
  vi.stubGlobal('navigator', {
    ...navigator,
    mediaDevices: { getUserMedia },
  });
}

describe('CameraScanner', () => {
  const originalBarcodeDetector = window.BarcodeDetector;
  const originalSecure = window.isSecureContext;
  const originalPlay = HTMLMediaElement.prototype.play;

  afterEach(() => {
    if (originalBarcodeDetector === undefined) {
      delete window.BarcodeDetector;
    } else {
      window.BarcodeDetector = originalBarcodeDetector;
    }
    HTMLMediaElement.prototype.play = originalPlay;
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: originalSecure });
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('stays closed until the user opts in, and does not request a camera', () => {
    const getUserMedia = vi.fn();
    installSecureCamera(getUserMedia);

    renderScanner({ onScan: vi.fn() });

    expect(screen.getByRole('button', { name: 'Scan with camera' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Camera preview')).not.toBeInTheDocument();
    expect(getUserMedia).not.toHaveBeenCalled();
  });

  it('offers the scan mode control when the camera cannot start', () => {
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: false });
    const onModeChange = vi.fn();
    renderScanner({ onScan: vi.fn(), mode: 'stock_out', onModeChange });

    fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));

    expect(screen.getByRole('radio', { name: 'STOCK OUT' })).toBeChecked();
    fireEvent.click(screen.getByRole('radio', { name: 'STOCK IN' }));
    expect(onModeChange).toHaveBeenCalledWith('stock_in');
    expect(screen.getByLabelText('Type a barcode')).toBeInTheDocument();
  });

  it('explains the HTTPS requirement and offers manual entry without opening a camera', () => {
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: false });
    const getUserMedia = vi.fn();
    vi.stubGlobal('navigator', { ...navigator, mediaDevices: { getUserMedia } });
    const onScan = vi.fn();
    renderScanner({ onScan });

    fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));

    expect(screen.getByText(/secure connection \(https\)/i)).toBeInTheDocument();
    expect(getUserMedia).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText('Type a barcode'), { target: { value: '0123456789012' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add scan' }));

    expect(onScan).toHaveBeenCalledWith('0123456789012');
  });

  it('does not call onScan when the typed barcode is blank', () => {
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: false });
    const onScan = vi.fn();
    renderScanner({ onScan });

    fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));
    fireEvent.click(screen.getByRole('button', { name: 'Add scan' }));

    expect(onScan).not.toHaveBeenCalled();
  });

  it('shows manual entry when no decoder is available and does not request a camera', async () => {
    const getUserMedia = vi.fn();
    installSecureCamera(getUserMedia);
    const createDecoder = vi.fn().mockResolvedValue(null);
    renderScanner({ onScan: vi.fn(), createDecoder });

    fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));

    expect(await screen.findByText(/not supported in this browser/i)).toBeInTheDocument();
    expect(screen.getByLabelText('Type a barcode')).toBeInTheDocument();
    expect(getUserMedia).not.toHaveBeenCalled();
  });

  describe('when the camera cannot start', () => {
    beforeEach(() => {
      Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true });
    });

    it('falls back to manual entry when permission is denied, and try again asks once more', async () => {
      const getUserMedia = vi.fn().mockRejectedValue(new DOMException('no', 'NotAllowedError'));
      vi.stubGlobal('navigator', { ...navigator, mediaDevices: { getUserMedia } });
      const decoder: FrameDecoder = { detect: vi.fn() };
      renderScanner({ onScan: vi.fn(), createDecoder: async () => decoder });

      fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));

      expect(await screen.findByText(/camera access was denied/i)).toBeInTheDocument();
      expect(screen.getByLabelText('Type a barcode')).toBeInTheDocument();

      fireEvent.click(screen.getByRole('button', { name: 'Try again' }));

      await vi.waitFor(() => expect(getUserMedia).toHaveBeenCalledTimes(2));
    });

    it('closes the fallback and returns to the opt-in button', async () => {
      const getUserMedia = vi.fn().mockRejectedValue(new DOMException('missing', 'NotFoundError'));
      vi.stubGlobal('navigator', { ...navigator, mediaDevices: { getUserMedia } });
      renderScanner({ onScan: vi.fn(), createDecoder: async () => ({ detect: vi.fn() }) });

      fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));
      expect(await screen.findByText(/no camera was found/i)).toBeInTheDocument();

      fireEvent.click(screen.getByRole('button', { name: 'Close' }));

      expect(screen.getByRole('button', { name: 'Scan with camera' })).toBeInTheDocument();
      expect(screen.queryByLabelText('Type a barcode')).not.toBeInTheDocument();
    });
  });

  describe('when the camera starts', () => {
    let getUserMedia: ReturnType<typeof vi.fn>;
    let stop: ReturnType<typeof vi.fn>;

    beforeEach(() => {
      Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true });
      stop = vi.fn();
      const stream = { getTracks: () => [{ stop }] } as unknown as MediaStream;
      getUserMedia = vi.fn().mockResolvedValue(stream);
      vi.stubGlobal('navigator', {
        ...navigator,
        mediaDevices: {
          getUserMedia,
          enumerateDevices: vi.fn().mockResolvedValue([
            { kind: 'videoinput', deviceId: 'rear' },
            { kind: 'videoinput', deviceId: 'front' },
          ]),
        },
      });
      HTMLMediaElement.prototype.play = vi.fn().mockResolvedValue(undefined);
    });

    function decoderReturning(values: string[]): FrameDecoder {
      const queue = [...values];
      return {
        detect: vi.fn().mockImplementation(async () => {
          const next = queue.shift();
          return next === undefined ? [] : [code(next)];
        }),
      };
    }

    function code(value: string, extras: Partial<DecodedBarcode> = {}): DecodedBarcode {
      return { value, corners: [], frameWidth: 0, frameHeight: 0, ...extras };
    }

    it('shows the preview and reports each distinct barcode', async () => {
      const onScan = vi.fn();
      renderScanner({ onScan, createDecoder: async () => decoderReturning(['111', '222']) });

      fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));

      expect(await screen.findByLabelText('Camera preview')).toBeInTheDocument();
      await vi.waitFor(() => expect(onScan).toHaveBeenCalledTimes(2));
      expect(onScan).toHaveBeenNthCalledWith(1, '111');
      expect(onScan).toHaveBeenNthCalledWith(2, '222');
      expect(screen.getByText('Captured 222')).toBeInTheDocument();
      expect(screen.queryByText(/scans automatically/i)).not.toBeInTheDocument();
      expect(document.querySelector('.camera-preview-flash')).not.toBeNull();
      expect(getUserMedia).toHaveBeenCalledWith({
        video: { facingMode: { ideal: 'environment' } },
        audio: false,
      });
    });

    it('ignores a repeat of the same barcode until the cooldown passes', async () => {
      let now = 10_000;
      vi.spyOn(Date, 'now').mockImplementation(() => now);
      let reads = 0;
      const detect = vi.fn().mockImplementation(async () => {
        reads += 1;
        if (reads >= 3) now = 10_000 + RESCAN_COOLDOWN_MS + 1;
        return [code('0123456789012')];
      });
      const onScan = vi.fn();
      renderScanner({ onScan, createDecoder: async () => ({ detect }) });

      fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));

      await vi.waitFor(() => expect(onScan).toHaveBeenCalledTimes(2));
      expect(onScan).toHaveBeenNthCalledWith(1, '0123456789012');
      expect(onScan).toHaveBeenNthCalledWith(2, '0123456789012');
    });

    it('stops the camera tracks when the user closes the preview', async () => {
      renderScanner({ onScan: vi.fn(), createDecoder: async () => decoderReturning([]) });

      fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));
      expect(await screen.findByLabelText('Camera preview')).toBeInTheDocument();

      fireEvent.click(screen.getByRole('button', { name: 'Stop camera' }));

      expect(stop).toHaveBeenCalled();
      expect(screen.getByRole('button', { name: 'Scan with camera' })).toBeInTheDocument();
      expect(screen.queryByLabelText('Camera preview')).not.toBeInTheDocument();
    });

    it('releases the camera when the scanner unmounts', async () => {
      const { unmount } = renderScanner({
        onScan: vi.fn(),
        createDecoder: async () => decoderReturning([]),
      });

      fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));
      expect(await screen.findByLabelText('Camera preview')).toBeInTheDocument();

      unmount();

      expect(stop).toHaveBeenCalled();
    });

    it('switches to the other facing camera', async () => {
      renderScanner({ onScan: vi.fn(), createDecoder: async () => decoderReturning([]) });

      fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));
      fireEvent.click(await screen.findByRole('button', { name: 'Switch camera' }));

      await vi.waitFor(() => expect(getUserMedia).toHaveBeenCalledTimes(2));
      expect(getUserMedia).toHaveBeenLastCalledWith({
        video: { facingMode: { ideal: 'user' } },
        audio: false,
      });
    });

    it('asks for a tap when the preview cannot start playing on its own', async () => {
      HTMLMediaElement.prototype.play = vi.fn()
        .mockRejectedValueOnce(new DOMException('gesture', 'NotAllowedError'))
        .mockResolvedValue(undefined);
      renderScanner({ onScan: vi.fn(), createDecoder: async () => decoderReturning([]) });

      fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));

      expect(await screen.findByRole('button', { name: 'Tap to start scanning' })).toBeInTheDocument();
      expect(stop).not.toHaveBeenCalled();

      fireEvent.click(screen.getByRole('button', { name: 'Tap to start scanning' }));

      expect(await screen.findByText(/scans automatically/i)).toBeInTheDocument();
      expect(screen.queryByRole('button', { name: 'Tap to start scanning' })).not.toBeInTheDocument();
    });

    it('draws an outline around a barcode the detector locates', async () => {
      const located = code('111', {
        corners: [
          { x: 10, y: 20 },
          { x: 80, y: 20 },
          { x: 80, y: 50 },
          { x: 10, y: 50 },
        ],
        frameWidth: 200,
        frameHeight: 100,
      });
      const { container } = renderScanner({
        onScan: vi.fn(),
        createDecoder: async () => ({ detect: vi.fn().mockResolvedValue([located]) }),
      });

      fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));

      const polygon = await vi.waitFor(() => {
        const node = container.querySelector('.camera-preview-boxes polygon');
        expect(node).not.toBeNull();
        return node;
      });
      expect(polygon).toHaveAttribute('points', '10,20 80,20 80,50 10,50');
      expect(container.querySelector('.camera-preview-boxes')).toHaveAttribute('viewBox', '0 0 200 100');
    });

    it('switches scan mode from the open preview without closing the camera', async () => {
      const onModeChange = vi.fn();
      const onScan = vi.fn();
      renderScanner({
        onScan,
        mode: 'stock_in',
        onModeChange,
        createDecoder: async () => decoderReturning([]),
      });

      expect(screen.queryByRole('radio', { name: 'STOCK IN' })).not.toBeInTheDocument();

      fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));

      expect(await screen.findByRole('radio', { name: 'STOCK IN' })).toBeChecked();
      fireEvent.click(screen.getByRole('radio', { name: 'STOCK OUT' }));

      expect(onModeChange).toHaveBeenCalledWith('stock_out');
      expect(onScan).not.toHaveBeenCalled();
      expect(screen.getByLabelText('Camera preview')).toBeInTheDocument();
      expect(screen.queryByRole('button', { name: 'Scan with camera' })).not.toBeInTheDocument();
    });

    it('sizes the picture from a wrapper so a phone can still see the queue', async () => {
      const { container } = renderScanner({ onScan: vi.fn(), createDecoder: async () => decoderReturning([]) });

      fireEvent.click(screen.getByRole('button', { name: 'Scan with camera' }));

      const video = await screen.findByLabelText('Camera preview');
      expect(video).toHaveClass('camera-preview-video');
      expect(container.querySelector('.camera-preview')).toContainElement(video);
      expect(await screen.findByText(/scans automatically/i)).toBeInTheDocument();
    });
  });
});

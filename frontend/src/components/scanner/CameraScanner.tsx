// Opt-in camera capture for phones and for desktops that have a webcam.
// The camera stays off until the user asks for it, and the same onScan
// callback the hardware scanner uses receives each barcode.
import { useCallback, useEffect, useRef, useState } from 'react';
import { Alert, Box, Button, Group, Loader, Stack, TextInput } from '@mantine/core';
import {
  CAMERA_START_TIMEOUT_MS,
  RESCAN_COOLDOWN_MS,
  SCAN_INTERVAL_MS,
  cameraBlockReason,
  describeCameraError,
  openCameraStream,
  shouldAcceptScan,
  type CameraBlock,
  type FacingMode,
  type PriorScan,
} from './cameraAccess';
import { outlineAttribute } from './cameraOverlay';
import { createFrameDecoder, DEFAULT_BARCODE_FORMATS, type DecodedBarcode, type FrameDecoder } from './frameDecoder';
import { ScannerModeSwitch } from './ScannerModeSwitch';
import type { ScanDirection } from '../../types';

export interface CameraScannerProps {
  onScan: (barcode: string) => void;
  onOpenChange?: (open: boolean) => void;
  // Shared scanner direction. When both are set, the open camera shows a
  // control that writes through the same path as the queue header.
  mode?: ScanDirection;
  onModeChange?: (mode: ScanDirection) => void;
  formats?: readonly string[];
  // Test seam. Production uses the native detector, then ZXing.
  createDecoder?: (formats: readonly string[]) => Promise<FrameDecoder | null>;
}

const CAPTURE_FLASH_MS = 700;

function CameraIcon() {
  return (
    <svg className="scan-camera-icon" viewBox="0 0 24 24" width="16" height="16" aria-hidden="true">
      <path
        fill="currentColor"
        d="M9.5 6.5 8.2 8H5.5A1.5 1.5 0 0 0 4 9.5v8A1.5 1.5 0 0 0 5.5 19h13a1.5 1.5 0 0 0 1.5-1.5v-8A1.5 1.5 0 0 0 18.5 8h-2.7l-1.3-1.5h-5Zm2.5 3.2a3.3 3.3 0 1 1 0 6.6 3.3 3.3 0 0 1 0-6.6Z"
      />
    </svg>
  );
}

export function CameraScanner({
  onScan,
  onOpenChange,
  mode,
  onModeChange,
  formats = DEFAULT_BARCODE_FORMATS,
  createDecoder = createFrameDecoder,
}: CameraScannerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const onScanRef = useRef(onScan);
  const onOpenChangeRef = useRef(onOpenChange);
  const flashTimer = useRef(0);
  useEffect(() => {
    onScanRef.current = onScan;
  }, [onScan]);
  useEffect(() => {
    onOpenChangeRef.current = onOpenChange;
  }, [onOpenChange]);

  const [active, setActive] = useState(false);
  const [block, setBlock] = useState<CameraBlock | null>(null);
  const [facing, setFacing] = useState<FacingMode>('environment');
  const [previewReady, setPreviewReady] = useState(false);
  const [needsTap, setNeedsTap] = useState(false);
  const [canSwitch, setCanSwitch] = useState(false);
  const [lastScan, setLastScan] = useState('');
  const [flash, setFlash] = useState(false);
  const [outline, setOutline] = useState<DecodedBarcode | null>(null);
  const [manualBarcode, setManualBarcode] = useState('');

  const markCaptured = useCallback((value: string) => {
    setLastScan(value);
    setFlash(true);
    window.clearTimeout(flashTimer.current);
    flashTimer.current = window.setTimeout(() => setFlash(false), CAPTURE_FLASH_MS);
    try {
      navigator.vibrate?.(40);
    } catch {
      // Vibration is a bonus cue; a missing or blocked API should not stop the scan.
    }
  }, []);

  useEffect(() => () => window.clearTimeout(flashTimer.current), []);

  useEffect(() => {
    onOpenChangeRef.current?.(active);
  }, [active]);

  useEffect(() => {
    if (!active || block !== null) return;

    let cancelled = false;
    let stream: MediaStream | null = null;
    let timer = 0;
    let last: PriorScan | null = null;

    const wait = (ms: number) => new Promise<void>((resolve) => {
      timer = window.setTimeout(resolve, ms);
    });

    const releaseStream = () => {
      window.clearTimeout(timer);
      stream?.getTracks().forEach((track) => track.stop());
      stream = null;
      if (videoRef.current) videoRef.current.srcObject = null;
    };

    void (async () => {
      let decoder: FrameDecoder | null;
      try {
        decoder = await createDecoder(formats);
      } catch {
        decoder = null;
      }
      if (cancelled) return;
      if (decoder === null) {
        setBlock({
          kind: 'unsupported',
          message: 'Barcode scanning is not supported in this browser. Type the barcode instead.',
        });
        return;
      }

      const pending = openCameraStream(facing);
      let timedOut = false;
      let opened: MediaStream;
      try {
        opened = await Promise.race([
          pending,
          new Promise<MediaStream>((_resolve, reject) => {
            timer = window.setTimeout(() => {
              timedOut = true;
              reject(new DOMException('The camera did not respond.', 'AbortError'));
            }, CAMERA_START_TIMEOUT_MS);
          }),
        ]);
      } catch (error) {
        // getUserMedia can still resolve after we gave up. Stop that stream so
        // the camera indicator does not stay on.
        if (timedOut) {
          void pending.then((late) => {
            late.getTracks().forEach((track) => track.stop());
          }).catch(() => undefined);
        }
        if (!cancelled) setBlock({ kind: 'camera', message: describeCameraError(error) });
        return;
      }
      window.clearTimeout(timer);
      if (cancelled) {
        opened.getTracks().forEach((track) => track.stop());
        return;
      }
      stream = opened;

      const video = videoRef.current;
      if (video === null) {
        releaseStream();
        if (!cancelled) setBlock({ kind: 'camera', message: 'The camera preview could not start.' });
        return;
      }
      video.srcObject = stream;
      video.muted = true;
      // iOS drops the click gesture during the camera prompt, so play() can
      // reject until the person taps the picture. Keep the stream and ask.
      try {
        await video.play();
        if (!cancelled) setNeedsTap(false);
      } catch (error) {
        const name = error instanceof DOMException ? error.name : '';
        if (name === 'NotAllowedError' || name === 'AbortError') {
          if (!cancelled) setNeedsTap(true);
        } else {
          releaseStream();
          if (!cancelled) setBlock({ kind: 'camera', message: 'The camera preview could not start.' });
          return;
        }
      }
      if (cancelled) {
        releaseStream();
        return;
      }
      setPreviewReady(true);

      try {
        const devices = await navigator.mediaDevices.enumerateDevices?.();
        const cameras = devices?.filter((device) => device.kind === 'videoinput').length ?? 0;
        if (!cancelled) setCanSwitch(cameras > 1);
      } catch {
        if (!cancelled) setCanSwitch(false);
      }

      while (!cancelled) {
        if (!document.hidden) {
          try {
            const hits = await decoder.detect(video);
            const hit = hits.find((entry) => entry.value !== '');
            const located = hit !== undefined && hit.frameWidth > 0 && hit.frameHeight > 0 && hit.corners.length >= 2
              ? hit
              : null;
            setOutline((current) => (located === null && current === null ? current : located));
            const now = Date.now();
            if (hit !== undefined && shouldAcceptScan(last, hit.value, now, RESCAN_COOLDOWN_MS)) {
              last = { value: hit.value, at: now };
              markCaptured(hit.value);
              onScanRef.current(hit.value);
            }
          } catch {
            // One undecodable frame is normal between successful reads.
          }
        }
        if (cancelled) break;
        await wait(SCAN_INTERVAL_MS);
      }
    })();

    return () => {
      cancelled = true;
      releaseStream();
    };
  }, [active, block, createDecoder, facing, formats, markCaptured]);

  function open() {
    const blocked = cameraBlockReason();
    setLastScan('');
    setFlash(false);
    setOutline(null);
    setPreviewReady(false);
    setNeedsTap(false);
    setCanSwitch(false);
    setManualBarcode('');
    setBlock(blocked);
    setActive(true);
  }

  function close() {
    setActive(false);
    setBlock(null);
    setPreviewReady(false);
    setNeedsTap(false);
    setCanSwitch(false);
    setLastScan('');
    setFlash(false);
    setOutline(null);
    setManualBarcode('');
  }

  function retry() {
    setPreviewReady(false);
    setNeedsTap(false);
    setCanSwitch(false);
    setOutline(null);
    setBlock(null);
  }

  function switchCamera() {
    setPreviewReady(false);
    setNeedsTap(false);
    setOutline(null);
    setFacing((current) => (current === 'environment' ? 'user' : 'environment'));
  }

  async function resumePreview() {
    const video = videoRef.current;
    if (video === null) return;
    try {
      await video.play();
      setNeedsTap(false);
    } catch {
      setNeedsTap(true);
    }
  }

  const modeSwitch = mode !== undefined && onModeChange !== undefined ? (
    <ScannerModeSwitch
      mode={mode}
      onChange={onModeChange}
      label="Camera scan mode"
      className="camera-mode-switch"
      size="lg"
    />
  ) : null;

  function submitManualBarcode(event: React.FormEvent) {
    event.preventDefault();
    const trimmed = manualBarcode.trim();
    if (trimmed === '') return;
    onScan(trimmed);
    setManualBarcode('');
  }

  if (!active) {
    return (
      <div className="camera-scanner-region" role="region" aria-label="Camera barcode scanner">
        <Button
          type="button"
          size="compact-sm"
          variant="light"
          leftSection={<CameraIcon />}
          aria-label="Scan with camera"
          onClick={open}
        >
          Camera
        </Button>
      </div>
    );
  }

  if (block !== null) {
    return (
      <Stack gap="sm" className="camera-scanner-region camera-scanner-fallback" role="region" aria-label="Camera barcode scanner">
        {modeSwitch}
        <Alert color="yellow" title="Camera scanning unavailable">
          {block.message}
        </Alert>
        <form onSubmit={submitManualBarcode}>
          <Group align="flex-end" wrap="wrap">
            <TextInput
              label="Type a barcode"
              placeholder="Enter barcode"
              value={manualBarcode}
              onChange={(event) => setManualBarcode(event.currentTarget.value)}
              style={{ flex: '1 1 12rem' }}
            />
            <Button type="submit">Add scan</Button>
          </Group>
        </form>
        <Group>
          {block.kind === 'camera' && (
            <Button type="button" variant="default" onClick={retry}>Try again</Button>
          )}
          <Button type="button" variant="default" onClick={close}>Close</Button>
        </Group>
      </Stack>
    );
  }

  const status = !previewReady
    ? 'Starting camera…'
    : lastScan !== ''
      ? `Captured ${lastScan}`
      : 'Hold a barcode in the frame. It scans automatically.';
  const outlinePoints = outline === null ? '' : outlineAttribute(outline.corners);

  return (
    <Stack gap="xs" className="camera-scanner-region" role="region" aria-label="Camera barcode scanner">
      <Box className={flash ? 'camera-preview camera-preview--captured' : 'camera-preview'}>
        <video
          ref={videoRef}
          className="camera-preview-video"
          muted
          playsInline
          autoPlay
          aria-label="Camera preview"
          onPlay={() => setNeedsTap(false)}
        />
        {outline !== null && outlinePoints !== '' && (
          <svg
            className="camera-preview-boxes"
            viewBox={`0 0 ${outline.frameWidth} ${outline.frameHeight}`}
            preserveAspectRatio="xMidYMid slice"
            aria-hidden="true"
          >
            <polygon points={outlinePoints} />
          </svg>
        )}
        {outline === null && previewReady && !needsTap && (
          <div className="camera-preview-reticle" aria-hidden="true" />
        )}
        {flash && <div className="camera-preview-flash" aria-hidden="true" />}
        {!previewReady && (
          <Box pos="absolute" top="50%" left="50%" style={{ transform: 'translate(-50%, -50%)' }}>
            <Loader aria-label="Starting camera" />
          </Box>
        )}
        {needsTap ? (
          <button type="button" className="camera-preview-tap" onClick={() => void resumePreview()}>
            Tap to start scanning
          </button>
        ) : (
          <p className={lastScan !== '' ? 'camera-preview-status camera-preview-status--captured' : 'camera-preview-status'} role="status" aria-live="polite">
            {status}
          </p>
        )}
      </Box>
      {modeSwitch}
      <Group>
        <Button type="button" variant="default" onClick={close}>Stop camera</Button>
        {canSwitch && (
          <Button type="button" variant="default" onClick={switchCamera}>Switch camera</Button>
        )}
      </Group>
    </Stack>
  );
}

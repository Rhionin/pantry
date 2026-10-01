// Opt-in camera capture for phones and for desktops that have a webcam.
// The camera stays off until the user asks for it, and the same onScan
// callback the hardware scanner uses receives each barcode.
import { useEffect, useRef, useState } from 'react';
import { Alert, Box, Button, Group, Loader, Stack, Text, TextInput } from '@mantine/core';
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
import { createFrameDecoder, DEFAULT_BARCODE_FORMATS, type FrameDecoder } from './frameDecoder';

export interface CameraScannerProps {
  onScan: (barcode: string) => void;
  formats?: readonly string[];
  // Test seam. Production uses the native detector, then ZXing.
  createDecoder?: (formats: readonly string[]) => Promise<FrameDecoder | null>;
}

export function CameraScanner({
  onScan,
  formats = DEFAULT_BARCODE_FORMATS,
  createDecoder = createFrameDecoder,
}: CameraScannerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const onScanRef = useRef(onScan);
  useEffect(() => {
    onScanRef.current = onScan;
  }, [onScan]);

  const [active, setActive] = useState(false);
  const [block, setBlock] = useState<CameraBlock | null>(null);
  const [facing, setFacing] = useState<FacingMode>('environment');
  const [previewReady, setPreviewReady] = useState(false);
  const [canSwitch, setCanSwitch] = useState(false);
  const [lastScan, setLastScan] = useState('');
  const [manualBarcode, setManualBarcode] = useState('');

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
      try {
        await video.play();
      } catch {
        releaseStream();
        if (!cancelled) setBlock({ kind: 'camera', message: 'The camera preview could not start.' });
        return;
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
            const values = await decoder.detect(video);
            const value = values.find((entry) => entry.trim() !== '')?.trim();
            const now = Date.now();
            if (value !== undefined && shouldAcceptScan(last, value, now, RESCAN_COOLDOWN_MS)) {
              last = { value, at: now };
              setLastScan(value);
              onScanRef.current(value);
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
  }, [active, block, createDecoder, facing, formats]);

  function open() {
    const blocked = cameraBlockReason();
    setLastScan('');
    setPreviewReady(false);
    setCanSwitch(false);
    setManualBarcode('');
    setBlock(blocked);
    setActive(true);
  }

  function close() {
    setActive(false);
    setBlock(null);
    setPreviewReady(false);
    setCanSwitch(false);
    setLastScan('');
    setManualBarcode('');
  }

  function retry() {
    setPreviewReady(false);
    setCanSwitch(false);
    setBlock(null);
  }

  function switchCamera() {
    setPreviewReady(false);
    setFacing((current) => (current === 'environment' ? 'user' : 'environment'));
  }

  function submitManualBarcode(event: React.FormEvent) {
    event.preventDefault();
    const trimmed = manualBarcode.trim();
    if (trimmed === '') return;
    onScan(trimmed);
    setManualBarcode('');
  }

  if (!active) {
    return (
      <Stack gap="xs" role="region" aria-label="Camera barcode scanner">
        <Button type="button" onClick={open} w={{ base: '100%', sm: 'auto' }}>
          Scan with camera
        </Button>
      </Stack>
    );
  }

  if (block !== null) {
    return (
      <Stack gap="sm" role="region" aria-label="Camera barcode scanner">
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

  return (
    <Stack gap="sm" role="region" aria-label="Camera barcode scanner">
      <Box pos="relative" maw={480}>
        <video
          ref={videoRef}
          muted
          playsInline
          autoPlay
          aria-label="Camera preview"
          style={{
            width: '100%',
            minHeight: 220,
            maxHeight: 360,
            objectFit: 'cover',
            borderRadius: 8,
            background: '#111',
          }}
        />
        <Box
          pos="absolute"
          top="15%"
          left="10%"
          w="80%"
          h="70%"
          style={{
            border: '3px solid var(--mantine-color-blue-5)',
            borderRadius: 8,
            pointerEvents: 'none',
          }}
        />
        {!previewReady && (
          <Box pos="absolute" top="50%" left="50%" style={{ transform: 'translate(-50%, -50%)' }}>
            <Loader aria-label="Starting camera" />
          </Box>
        )}
      </Box>
      <Text size="sm" c="dimmed" aria-live="polite">
        {lastScan !== ''
          ? `Scanned ${lastScan}`
          : previewReady
            ? 'Point the camera at a barcode.'
            : 'Starting camera…'}
      </Text>
      <Group>
        <Button type="button" variant="default" onClick={close}>Stop camera</Button>
        {canSwitch && (
          <Button type="button" variant="default" onClick={switchCamera}>Switch camera</Button>
        )}
      </Group>
    </Stack>
  );
}

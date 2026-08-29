// Camera-based barcode scanner using the browser BarcodeDetector API.
// The BarcodeDetector API has no TypeScript lib definitions yet, so the
// minimal shape used here is declared locally.
import { useEffect, useRef, useState } from 'react';
import { Alert, Box, Button, Group, Stack, Text, TextInput } from '@mantine/core';

interface BarcodeDetectorResult {
  rawValue: string;
}

interface BarcodeDetectorInstance {
  detect: (source: HTMLVideoElement) => Promise<BarcodeDetectorResult[]>;
}

interface BarcodeDetectorConstructor {
  new (options?: { formats: string[] }): BarcodeDetectorInstance;
}

declare global {
  interface Window {
    BarcodeDetector?: BarcodeDetectorConstructor;
  }
}

export interface CameraScannerProps {
  onScan: (barcode: string) => void;
  formats?: string[];
}

const DEFAULT_FORMATS = ['ean_13', 'ean_8', 'upc_a', 'upc_e', 'code_128'];

// Same barcode detected again within this window is treated as a re-read of
// the item still in frame, not a second scan.
const RESCAN_COOLDOWN_MS = 2000;

export function CameraScanner({ onScan, formats = DEFAULT_FORMATS }: CameraScannerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const frameRef = useRef<number | null>(null);
  const lastScanRef = useRef<{ value: string; at: number } | null>(null);
  const [manualBarcode, setManualBarcode] = useState('');
  // Support is a fixed characteristic of the browser, not state that changes
  // during the component's lifetime, so it is computed at render time
  // rather than set from inside an effect.
  const isSupported = typeof window.BarcodeDetector !== 'undefined';
  const [fallbackReason, setFallbackReason] = useState<string | null>(
    isSupported ? null : 'Barcode scanning is not supported in this browser.',
  );

  useEffect(() => {
    if (typeof window.BarcodeDetector === 'undefined') {
      return;
    }

    let cancelled = false;
    const detector = new window.BarcodeDetector({ formats });

    async function scanFrame() {
      if (cancelled || !videoRef.current) {
        return;
      }
      try {
        const results = await detector.detect(videoRef.current);
        if (results.length > 0) {
          const value = results[0].rawValue;
          const now = Date.now();
          const last = lastScanRef.current;
          if (!last || last.value !== value || now - last.at > RESCAN_COOLDOWN_MS) {
            lastScanRef.current = { value, at: now };
            onScan(value);
          }
        }
      } catch {
        // Transient decode errors are expected between frames; keep polling.
      }
      if (!cancelled) {
        frameRef.current = requestAnimationFrame(() => {
          void scanFrame();
        });
      }
    }

    async function start() {
      try {
        const stream = await navigator.mediaDevices.getUserMedia({
          video: { facingMode: 'environment' },
        });
        if (cancelled) {
          stream.getTracks().forEach((track) => track.stop());
          return;
        }
        streamRef.current = stream;
        if (videoRef.current) {
          videoRef.current.srcObject = stream;
          await videoRef.current.play();
        }
        void scanFrame();
      } catch {
        if (!cancelled) {
          setFallbackReason('Camera access was denied or is unavailable.');
        }
      }
    }

    void start();

    return () => {
      cancelled = true;
      if (frameRef.current !== null) {
        cancelAnimationFrame(frameRef.current);
      }
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
    };
  }, [formats, onScan, isSupported]);

  function submitManualBarcode(event: React.FormEvent) {
    event.preventDefault();
    const trimmed = manualBarcode.trim();
    if (trimmed) {
      onScan(trimmed);
      setManualBarcode('');
    }
  }

  if (fallbackReason) {
    return (
      <Stack gap="sm">
        <Alert color="yellow" title="Camera scanning unavailable">
          {fallbackReason} Enter the barcode manually instead.
        </Alert>
        <form onSubmit={submitManualBarcode}>
          <Group align="flex-end">
            <TextInput
              aria-label="Barcode"
              placeholder="Enter barcode"
              value={manualBarcode}
              onChange={(event) => setManualBarcode(event.currentTarget.value)}
            />
            <Button type="submit">Submit</Button>
          </Group>
        </form>
      </Stack>
    );
  }

  return (
    <Box pos="relative" maw={480}>
      <video ref={videoRef} muted playsInline style={{ width: '100%', borderRadius: 8 }}>
        <track kind="captions" />
      </video>
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
      <Text size="sm" c="dimmed" mt="xs">
        Point the camera at a barcode.
      </Text>
    </Box>
  );
}

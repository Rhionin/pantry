// A detected code is drawn as a quadrilateral in the frame's pixel space.
// Native detectors report four corners. A 1D decoder often reports only the
// two ends of the bars, which we expand into a band across the code.

export interface Point {
  x: number;
  y: number;
}

export function barcodeOutline(corners: readonly Point[]): Point[] {
  if (corners.length >= 4) {
    return corners.slice(0, 4).map((point) => ({ x: point.x, y: point.y }));
  }
  if (corners.length === 3) return axisAlignedBox(corners, 0.12);
  if (corners.length === 2) return spanBox(corners[0], corners[1]);
  return [];
}

export function outlineAttribute(corners: readonly Point[]): string {
  return barcodeOutline(corners).map((point) => `${point.x},${point.y}`).join(' ');
}

function axisAlignedBox(points: readonly Point[], padRatio: number): Point[] {
  const xs = points.map((point) => point.x);
  const ys = points.map((point) => point.y);
  const minX = Math.min(...xs);
  const maxX = Math.max(...xs);
  const minY = Math.min(...ys);
  const maxY = Math.max(...ys);
  const padX = Math.max(2, (maxX - minX) * padRatio);
  const padY = Math.max(2, (maxY - minY) * padRatio);
  return [
    { x: minX - padX, y: minY - padY },
    { x: maxX + padX, y: minY - padY },
    { x: maxX + padX, y: maxY + padY },
    { x: minX - padX, y: maxY + padY },
  ];
}

function spanBox(start: Point, end: Point): Point[] {
  const dx = end.x - start.x;
  const dy = end.y - start.y;
  const length = Math.hypot(dx, dy);
  if (length === 0) return axisAlignedBox([start], 0);
  const pad = Math.max(6, length * 0.22);
  const px = (-dy / length) * pad;
  const py = (dx / length) * pad;
  return [
    { x: start.x + px, y: start.y + py },
    { x: end.x + px, y: end.y + py },
    { x: end.x - px, y: end.y - py },
    { x: start.x - px, y: start.y - py },
  ];
}

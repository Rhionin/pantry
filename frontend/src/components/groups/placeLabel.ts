export type LabelRegion = 'fill' | 'empty';

export interface LabelPlacement {
  left: number;
  fontSize: number;
  region: LabelRegion;
}

// The label stays entirely in the yellow or entirely in the empty side.
// Prefer the yellow when the measured line fits there with padding; otherwise
// start it just past the fill, still padded off the bar's far edge.
export function placeOnHandLabel({
  wellWidth,
  fillWidth,
  padding,
  sizes,
  textWidth,
}: {
  wellWidth: number;
  fillWidth: number;
  padding: number;
  sizes: number[];
  textWidth: (fontSize: number) => number;
}): LabelPlacement {
  const width = Math.max(0, wellWidth);
  const fill = Math.min(width, Math.max(0, fillWidth));
  const empty = width - fill;
  const steps = sizes.length > 0 ? sizes : [14];
  const fits = (region: number, line: number) => line + padding * 2 <= region + 0.5;

  for (const fontSize of steps) {
    const line = textWidth(fontSize);
    if (fill > 0 && fits(fill, line)) {
      return { left: padding, fontSize, region: 'fill' };
    }
    if (fits(empty, line)) {
      return { left: fill + padding, fontSize, region: 'empty' };
    }
  }

  const region: LabelRegion = fill >= empty && fill > 0 ? 'fill' : 'empty';
  const regionWidth = region === 'fill' ? fill : empty;
  const smallest = steps[steps.length - 1] ?? 14;
  const line = textWidth(smallest);
  const room = Math.max(0, regionWidth - padding * 2);
  const fontSize = line > room && line > 0 ? smallest * (room / line) : smallest;
  return {
    left: region === 'fill' ? padding : fill + padding,
    fontSize,
    region,
  };
}

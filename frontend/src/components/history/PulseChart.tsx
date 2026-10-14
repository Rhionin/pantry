import { useState } from 'react';
import type { LedgerMove } from '../../types';
import {
  buildPulseSeries, pulseEmptyCopy, pulseLegend, pulseRanges, pulseSidePaths, pulseStartLabel, pulseSummary,
  type PlotPoint, type PulseRange, type PulseSample,
} from './pulse';

const plotWidth = 1000;
const plotHeight = 280;
const baseline = 140;
const amplitude = 112;

export function PulseChart({ moves, now }: { moves: LedgerMove[]; now: Date }) {
  const [range, setRange] = useState<PulseRange>(30);
  const series = buildPulseSeries(moves, now, range);
  const summary = pulseSummary({
    rangeDays: range,
    stockedIn: series.centersIn.reduce((sum, center) => sum + center.quantity, 0),
    used: series.centersOut.reduce((sum, center) => sum + center.quantity, 0),
  });
  const startMs = series.samples[0]?.at ?? now.getTime();
  const endMs = series.samples[series.samples.length - 1]?.at ?? startMs;
  const span = endMs - startMs || 1;
  const scale = Math.max(series.peakIn, series.peakOut);
  const stockedIn = sidePoints(series.samples, 'stockedIn', -1, startMs, span, scale);
  const used = sidePoints(series.samples, 'used', 1, startMs, span, scale);
  const above = pulseSidePaths(stockedIn, baseline);
  const below = pulseSidePaths(used, baseline);

  return (
    <figure className="pulse">
      <div className="pulse-range" role="group" aria-label="Chart range">
        {pulseRanges.map((days) => (
          <button
            key={days}
            type="button"
            className="pulse-range-button"
            aria-pressed={range === days}
            onClick={() => setRange(days)}
          >
            {days} days
          </button>
        ))}
      </div>
      <svg
        className="pulse-plot"
        viewBox={`0 0 ${plotWidth} ${plotHeight}`}
        preserveAspectRatio="none"
        role="img"
        aria-label={summary}
      >
        {above && (
          <g>
            <path className="pulse-hump" d={above.fill} fill="var(--history-in)" fillOpacity={0.5} />
            <path d={above.stroke} fill="none" stroke="var(--history-in)" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
          </g>
        )}
        {below && (
          <g>
            <path className="pulse-hump" d={below.fill} fill="var(--history-used)" fillOpacity={0.5} />
            <path d={below.stroke} fill="none" stroke="var(--history-used)" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
          </g>
        )}
        <line className="pulse-baseline" x1={0} y1={baseline} x2={plotWidth} y2={baseline} />
      </svg>
      <div className="pulse-axis">
        <span>{pulseStartLabel(new Date(startMs), now)}</span>
        <span>Today</span>
      </div>
      {series.peakIn === 0 && series.peakOut === 0 && <p className="pulse-empty">{pulseEmptyCopy(range)}</p>}
      <p className="pulse-legend">{pulseLegend()}</p>
    </figure>
  );
}

function sidePoints(
  samples: PulseSample[],
  key: 'stockedIn' | 'used',
  sign: -1 | 1,
  startMs: number,
  span: number,
  scale: number,
): PlotPoint[] {
  return samples.map((sample) => ({
    x: ((sample.at - startMs) / span) * plotWidth,
    y: scale === 0 ? baseline : baseline + sign * (sample[key] / scale) * amplitude,
  }));
}

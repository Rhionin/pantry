import { useState } from 'react';
import type { LedgerMove } from '../../types';
import {
  buildPulseModel, pulseEmptyCopy, pulseLegend, pulseRanges, pulseStartLabel, pulseSummary,
  type PulseRange,
} from './pulse';

const plotWidth = 1000;
const plotHeight = 280;
const baseline = 140;
const amplitude = 112;

export function PulseChart({ moves, now }: { moves: LedgerMove[]; now: Date }) {
  const [range, setRange] = useState<PulseRange>(30);
  const model = buildPulseModel(moves, now, range);
  const summary = pulseSummary(model);
  // One day is only a few pixels wide on a phone, so each pulse is wider than
  // a day. Every pulse has the same width. The height carries the amount.
  const half = Math.min((plotWidth / range) * 2.05, plotWidth * 0.16);
  const startMs = model.start.getTime();
  const endMs = model.end.getTime();
  const span = endMs - startMs || 1;

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
        {[...model.marks].sort((left, right) => right.quantity - left.quantity).map((mark) => {
          const raw = ((mark.at - startMs) / span) * plotWidth;
          const x = Math.min(plotWidth - half, Math.max(half, raw));
          const y = baseline - mark.magnitude * amplitude;
          const tone = mark.direction === 'in' ? 'var(--history-in)' : 'var(--history-used)';
          return (
            <g key={`${mark.at}-${mark.direction}`}>
              <polygon
                points={`${x - half},${baseline} ${x},${y} ${x + half},${baseline}`}
                fill={tone}
                fillOpacity={0.5}
              />
              <line
                x1={x}
                y1={baseline}
                x2={x}
                y2={y}
                stroke={tone}
                strokeWidth={1.75}
                strokeLinecap="round"
                vectorEffect="non-scaling-stroke"
              />
            </g>
          );
        })}
        <line className="pulse-baseline" x1={0} y1={baseline} x2={plotWidth} y2={baseline} />
      </svg>
      <div className="pulse-axis">
        <span>{pulseStartLabel(model.start, now)}</span>
        <span>Today</span>
      </div>
      {model.marks.length === 0 && <p className="pulse-empty">{pulseEmptyCopy(range)}</p>}
      <p className="pulse-legend">{pulseLegend(model)}</p>
    </figure>
  );
}

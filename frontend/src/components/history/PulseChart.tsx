import { useEffect, useRef, useState } from 'react';
import type { LedgerMove } from '../../types';
import {
  buildPulseSeries, pulseCount, pulseDateTicks, pulseDayFor, pulseDays, pulseEmptyCopy, pulseLegend,
  pulsePlot, pulseRanges, pulseSidePaths, pulseStartLabel, pulseSummary, pulseTickLabel, pulseYTicks,
  type PlotPoint, type PulseDay, type PulseRange, type PulseSample,
} from './pulse';

const { width: plotWidth, height: plotHeight, baseline, amplitude } = pulsePlot;

export interface PulseInspect {
  key: string;
  scroll: boolean;
}

interface InspectState {
  ratio: number;
  day: PulseDay;
  locked: boolean;
}

export function PulseChart({
  moves, now, setupAt, onInspect,
}: {
  moves: LedgerMove[];
  now: Date;
  setupAt?: string;
  onInspect?: (day: PulseInspect | null) => void;
}) {
  const [range, setRange] = useState<PulseRange>(30);
  const [inspect, setInspect] = useState<{ state: InspectState; signature: string } | null>(null);
  const figureRef = useRef<HTMLElement>(null);
  const gesture = useRef<{ x: number; y: number; moved: boolean; scrolling: boolean } | null>(null);
  const signature = `${range}|${setupAt ?? ''}|${moves.map((move) => `${move.id}:${move.quantity}:${move.at}`).join()}`;
  const active = inspect?.signature === signature ? inspect.state : null;

  const series = buildPulseSeries(moves, now, range, setupAt);
  const since = series.fromSetup ? pulseStartLabel(series.start, now) : undefined;
  const summary = pulseSummary({
    rangeDays: range,
    stockedIn: series.stockedIn,
    used: series.used,
    since,
  });
  const startMs = series.start.getTime();
  const endMs = series.end.getTime();
  const span = endMs - startMs || 1;
  const scale = Math.max(series.peakIn, series.peakOut);
  const stockedIn = sidePoints(series.samples, 'stockedIn', -1, startMs, span, scale);
  const used = sidePoints(series.samples, 'used', 1, startMs, span, scale);
  const above = pulseSidePaths(stockedIn, baseline);
  const below = pulseSidePaths(used, baseline);
  const days = pulseDays(moves, series.start, series.end, now);
  const dateTicks = pulseDateTicks(series.start, series.end, now);
  const yTicks = pulseYTicks(scale);
  const dayIndex = active ? Math.max(0, days.findIndex((day) => day.key === active.day.key)) : 0;

  useEffect(() => {
    if (!active?.locked) return undefined;
    const close = (event: PointerEvent) => {
      if (!figureRef.current?.contains(event.target as Node)) {
        setInspect(null);
        onInspect?.(null);
      }
    };
    document.addEventListener('pointerdown', close);
    return () => document.removeEventListener('pointerdown', close);
  }, [active?.locked, onInspect]);

  const showDay = (clientX: number, target: HTMLDivElement, scroll: boolean, lock: boolean) => {
    const rect = target.getBoundingClientRect();
    const ratio = Math.min(1, Math.max(0, (clientX - rect.left) / (rect.width || 1)));
    const day = pulseDayFor(days, startMs + ratio * span);
    if (!day) return;
    setInspect({ signature, state: { ratio, day, locked: lock } });
    onInspect?.({ key: day.key, scroll });
  };

  const align = tipAlign(active?.ratio ?? 0.5);

  return (
    <figure className="pulse" ref={figureRef}>
      <div className="pulse-range" role="group" aria-label="Chart range">
        {pulseRanges.map((daysOption) => (
          <button
            key={daysOption}
            type="button"
            className="pulse-range-button"
            aria-pressed={range === daysOption}
            onClick={() => {
              setRange(daysOption);
              setInspect(null);
              onInspect?.(null);
            }}
          >
            {daysOption} days
          </button>
        ))}
      </div>
      <p className="pulse-totals" aria-hidden="true">
        <span className="pulse-total is-in">{pulseCount(series.stockedIn, 'stocked in')}</span>
        <span className="pulse-total is-out">{pulseCount(series.used, 'used')}</span>
      </p>
      <div className="pulse-stage">
        <div className="pulse-y" aria-hidden="true">
          {yTicks.map((tick) => (
            <span
              key={`${tick.y}`}
              className="pulse-y-tick"
              style={{ top: `${(tick.y / plotHeight) * 100}%` }}
            >
              {pulseTickLabel(tick.value)}
            </span>
          ))}
        </div>
        <div className="pulse-canvas">
          <svg
            className="pulse-plot"
            viewBox={`0 0 ${plotWidth} ${plotHeight}`}
            preserveAspectRatio="none"
            role="img"
            aria-label={summary}
          >
            {yTicks.filter((tick) => tick.value !== 0).map((tick) => (
              <line key={`grid-${tick.y}`} className="pulse-grid" x1={0} y1={tick.y} x2={plotWidth} y2={tick.y} />
            ))}
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
            {active && (
              <line
                className="pulse-crosshair"
                x1={active.ratio * plotWidth}
                x2={active.ratio * plotWidth}
                y1={8}
                y2={plotHeight - 8}
              />
            )}
          </svg>
          <div
            className="pulse-hit"
            role="slider"
            tabIndex={0}
            aria-label="Day on the chart"
            aria-valuemin={0}
            aria-valuemax={Math.max(0, days.length - 1)}
            aria-valuenow={dayIndex}
            aria-valuetext={active
              ? `${active.day.label}. ${pulseCount(active.day.stockedIn, 'stocked in')}. ${pulseCount(active.day.used, 'used')}.`
              : 'Move across the chart to read a day.'}
            onPointerDown={(event) => {
              if (event.pointerType === 'mouse' && event.button !== 0) return;
              gesture.current = { x: event.clientX, y: event.clientY, moved: false, scrolling: false };
              if (event.pointerType === 'mouse') showDay(event.clientX, event.currentTarget, false, false);
            }}
            onPointerMove={(event) => {
              if (event.pointerType === 'mouse') {
                showDay(event.clientX, event.currentTarget, false, false);
                return;
              }
              const start = gesture.current;
              if (!start || start.scrolling) return;
              const dx = event.clientX - start.x;
              const dy = event.clientY - start.y;
              if (!start.moved && Math.hypot(dx, dy) < 10) return;
              start.moved = true;
              if (Math.abs(dy) > Math.abs(dx)) {
                start.scrolling = true;
                return;
              }
              showDay(event.clientX, event.currentTarget, false, false);
            }}
            onPointerUp={(event) => {
              if (event.pointerType === 'mouse') return;
              const start = gesture.current;
              gesture.current = null;
              if (!start || start.scrolling) return;
              showDay(event.clientX, event.currentTarget, true, true);
            }}
            onPointerLeave={(event) => {
              if (event.pointerType !== 'mouse' || active?.locked) return;
              setInspect(null);
              onInspect?.(null);
            }}
            onPointerCancel={() => {
              gesture.current = null;
              if (active?.locked) return;
              setInspect(null);
              onInspect?.(null);
            }}
            onKeyDown={(event) => {
              if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight' && event.key !== 'Home' && event.key !== 'End') return;
              event.preventDefault();
              if (days.length === 0) return;
              const current = active ? days.findIndex((day) => day.key === active.day.key) : -1;
              let next = current;
              if (event.key === 'ArrowRight') next = Math.min(days.length - 1, current + 1);
              if (event.key === 'ArrowLeft') next = Math.max(0, current < 0 ? 0 : current - 1);
              if (event.key === 'Home') next = 0;
              if (event.key === 'End') next = days.length - 1;
              const day = days[next];
              const at = Math.min(endMs, Math.max(startMs, day.at + 12 * 60 * 60 * 1000));
              setInspect({ signature, state: { ratio: (at - startMs) / span, day, locked: true } });
              onInspect?.({ key: day.key, scroll: true });
            }}
          />
          {active && (
            <div className="pulse-tip" role="tooltip" data-align={align} style={{ left: `${active.ratio * 100}%` }}>
              <p className="pulse-tip-date">{active.day.label}</p>
              <p className="pulse-tip-line pulse-tip-in">{pulseCount(active.day.stockedIn, 'stocked in')}</p>
              <p className="pulse-tip-line pulse-tip-out">{pulseCount(active.day.used, 'used')}</p>
            </div>
          )}
        </div>
      </div>
      <div className="pulse-dates" aria-hidden="true">
        <span />
        <div className="pulse-dates-track">
          {dateTicks.map((tick) => (
            <span
              key={`${tick.align}-${tick.at}`}
              className="pulse-date"
              data-align={tick.align}
              style={tick.align === 'center' ? { left: `${((tick.at - startMs) / span) * 100}%` } : undefined}
            >
              {tick.label}
            </span>
          ))}
        </div>
      </div>
      {series.peakIn === 0 && series.peakOut === 0 && (
        <p className="pulse-empty">{pulseEmptyCopy(range, series.fromSetup)}</p>
      )}
      {series.fromSetup && <p className="pulse-origin">The opening stock in is where this chart starts.</p>}
      <p className="pulse-legend">{pulseLegend()}</p>
    </figure>
  );
}

function tipAlign(ratio: number): 'start' | 'center' | 'end' {
  if (ratio < 0.22) return 'start';
  if (ratio > 0.78) return 'end';
  return 'center';
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

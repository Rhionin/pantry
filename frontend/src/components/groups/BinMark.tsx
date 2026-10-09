import { useLayoutEffect, useRef, useState } from 'react';
import { ActionIcon, Title, Tooltip } from '@mantine/core';
import { binColor, type BinView } from './copy';
import { placeOnHandLabel, type LabelPlacement } from './placeLabel';
import './groups.css';

const labelSizes = (root: number, scale: number) => {
  const preferred = 0.875 * root * scale;
  return [preferred, preferred - 1, preferred - 2, Math.max(11, preferred - 3)];
};

function themePadding(): number {
  const style = getComputedStyle(document.documentElement);
  const root = parseFloat(style.fontSize) || 16;
  const scale = parseFloat(style.getPropertyValue('--mantine-scale')) || 1;
  return 0.625 * root * scale;
}

function PencilIcon() {
  return (
    <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true">
      <path
        fill="currentColor"
        d="M11.1 1.6a1.5 1.5 0 0 1 2.1 2.1L6.4 10.5l-2.7.8.8-2.7 6.6-7Z"
      />
      <path fill="currentColor" d="M2.2 13.1h11.6V14.4H2.2z" />
    </svg>
  );
}

function OnHandLabel({ text, percent }: { text: string; percent: number }) {
  const ref = useRef<HTMLParagraphElement>(null);
  const [place, setPlace] = useState<LabelPlacement | null>(null);

  useLayoutEffect(() => {
    const el = ref.current;
    const well = el?.offsetParent instanceof HTMLElement ? el.offsetParent : null;
    if (!el || !well) return;

    const measure = () => {
      const padding = themePadding();
      const root = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
      const scale = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--mantine-scale')) || 1;
      const fill = well.querySelector('.bin-fill');
      const fillWidth = fill instanceof HTMLElement ? fill.getBoundingClientRect().width : 0;
      let next = placeOnHandLabel({
        wellWidth: well.clientWidth,
        fillWidth,
        padding,
        sizes: labelSizes(root, scale),
        textWidth: (fontSize) => {
          el.style.fontSize = `${fontSize}px`;
          return el.scrollWidth;
        },
      });
      el.style.fontSize = `${next.fontSize}px`;
      const regionWidth = next.region === 'fill' ? fillWidth : well.clientWidth - fillWidth;
      let line = el.scrollWidth;
      let size = next.fontSize;
      while (line + padding * 2 > regionWidth + 0.5 && size > 8) {
        size -= 0.5;
        el.style.fontSize = `${size}px`;
        line = el.scrollWidth;
      }
      if (size !== next.fontSize) next = { ...next, fontSize: size };
      setPlace((current) => (
        current
        && current.left === next.left
        && current.fontSize === next.fontSize
        && current.region === next.region
          ? current
          : next
      ));
    };

    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(well);
    return () => observer.disconnect();
  }, [text, percent]);

  return (
    <p
      ref={ref}
      className={place ? `bin-caption is-in-${place.region}` : 'bin-caption'}
      style={{
        left: place ? place.left : 0,
        fontSize: place ? place.fontSize : undefined,
        visibility: place ? 'visible' : 'hidden',
      }}
    >
      {text}
    </p>
  );
}

export function BinMark({
  view,
  members,
  size,
  name,
  onRename,
  caption,
}: {
  view: BinView;
  members: { productId: string }[];
  size: 'hero' | 'mark';
  name?: string;
  onRename?: () => void;
  caption?: string;
}) {
  const percent = view.segments.reduce((sum, segment) => sum + segment.fraction, 0) * 100;
  const across = size === 'hero';
  const segments = view.segments.map((segment) => (
    <div
      key={segment.productId}
      className="bin-segment"
      style={{
        flexGrow: percent > 0 ? segment.fraction / (percent / 100) : 1,
        flexBasis: 0,
        background: binColor(members, segment.productId),
      }}
    />
  ));
  const fill = segments.length > 0 && (
    <div
      className={across ? 'bin-fill bin-fill-across' : 'bin-fill'}
      style={across ? { width: `${percent}%` } : { height: `${percent}%` }}
      aria-hidden="true"
    >
      {segments}
    </div>
  );

  if (size === 'mark') {
    return (
      <div className="bin bin-mark" aria-hidden="true">
        <div className="bin-well">{fill}</div>
      </div>
    );
  }

  return (
    <div className="bin bin-hero">
      <div className="bin-head">
        <div className="bin-head-name">
          <Title order={1} className="bin-name">{name}</Title>
          {onRename && (
            <Tooltip label="Rename">
              <ActionIcon
                variant="subtle"
                color="dark"
                className="bin-icon-button"
                aria-label="Rename"
                onClick={onRename}
              >
                <PencilIcon />
              </ActionIcon>
            </Tooltip>
          )}
        </div>
        {caption === undefined && <p className="bin-corner">{view.corner}</p>}
      </div>
      <div className="bin-well">
        {fill}
        {caption !== undefined
          ? (caption !== '' && <OnHandLabel text={caption} percent={percent} />)
          : (view.level !== '' && <p className="bin-level">{view.level}</p>)}
      </div>
    </div>
  );
}

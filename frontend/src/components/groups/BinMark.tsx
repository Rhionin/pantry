import { Anchor, Title } from '@mantine/core';
import { binColor, type BinView } from './copy';
import './groups.css';

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
  const well = (
    <div className="bin-well" aria-hidden="true">
      {view.segments.length > 0 && (
        <div className="bin-fill" style={{ height: `${percent}%` }}>
          {view.segments.map((segment) => (
            <div
              key={segment.productId}
              className="bin-segment"
              style={{ flexGrow: segment.fraction, background: binColor(members, segment.productId) }}
            />
          ))}
        </div>
      )}
    </div>
  );

  if (size === 'mark') {
    return <div className="bin bin-mark" aria-hidden="true">{well}</div>;
  }

  return (
    <div className="bin bin-hero">
      {well}
      <div className="bin-head">
        <div className="bin-head-name">
          <Title order={1} className="bin-name">{name}</Title>
          {onRename && (
            <Anchor component="button" type="button" className="bin-action" onClick={onRename}>
              Rename
            </Anchor>
          )}
        </div>
        {caption === undefined && <p className="bin-corner">{view.corner}</p>}
      </div>
      {caption !== undefined
        ? (caption !== '' && <p className="bin-caption">{caption}</p>)
        : (view.level !== '' && <p className="bin-level">{view.level}</p>)}
    </div>
  );
}

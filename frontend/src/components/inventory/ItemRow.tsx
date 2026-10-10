import { memo, useRef, type ReactNode } from 'react';
import { ActionIcon, Badge, Checkbox, Menu } from '@mantine/core';
import type { GroupSuggestion, InventoryItem } from '../../types';
import { ProvenanceBadge } from '../product/ProvenanceBadge';
import { visibleCategory } from './inventoryUtils';
import { ProductPhoto } from './ProductPhoto';

export interface ItemRowProps {
  inventoryItem: InventoryItem;
  hand: string;
  suggestion?: GroupSuggestion | null;
  selected: boolean;
  controlsId: string;
  onSelect: () => void;
  onAdd: () => void;
  onStartGroup: () => void;
  onOpenSuggestion?: () => void;
  selecting?: boolean;
  checked?: boolean;
  onChecked?: (checked: boolean) => void;
  onLongPress?: () => void;
  children?: ReactNode;
}

function KebabIcon() {
  return (
    <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true">
      <circle cx="8" cy="3.1" r="1.35" fill="currentColor" />
      <circle cx="8" cy="8" r="1.35" fill="currentColor" />
      <circle cx="8" cy="12.9" r="1.35" fill="currentColor" />
    </svg>
  );
}

export const ItemRow = memo(({
  inventoryItem,
  hand,
  suggestion = null,
  selected,
  controlsId,
  onSelect,
  onAdd,
  onStartGroup,
  onOpenSuggestion,
  selecting = false,
  checked = false,
  onChecked,
  onLongPress,
  children,
}: ItemRowProps) => {
  const { item, nearExpiryCount, expiredCount } = inventoryItem;
  const category = visibleCategory(item.product.category);
  const hold = useRef<number | null>(null);

  const startHold = () => {
    if (!onLongPress || selecting) return;
    hold.current = window.setTimeout(() => onLongPress(), 500);
  };
  const clearHold = () => {
    if (hold.current !== null) {
      window.clearTimeout(hold.current);
      hold.current = null;
    }
  };

  return (
    <article
      className="shelf-row"
      onPointerDown={startHold}
      onPointerUp={clearHold}
      onPointerLeave={clearHold}
      onPointerCancel={clearHold}
    >
      <div className="shelf-head">
        {selecting && (
          <Checkbox
            aria-label={`Select ${item.product.name}`}
            checked={checked}
            onChange={(event) => onChecked?.(event.currentTarget.checked)}
            onPointerDown={(event) => event.stopPropagation()}
          />
        )}
        <ProductPhoto src={item.product.imageUrl} name={item.product.name} />
        <div className="shelf-copy">
          <h3 className="shelf-name">{item.product.name}</h3>
          {category !== null && <p className="shelf-category">{category}</p>}
          <div className="shelf-meta">
            <span>{hand}</span>
            {nearExpiryCount > 0 && <Badge size="sm" color="yellow">{nearExpiryCount} near expiry</Badge>}
            {expiredCount > 0 && <Badge size="sm" color="red">{expiredCount} expired</Badge>}
          </div>
          <ProvenanceBadge quiet externalSource={item.product.externalSource} />
          {suggestion && (
            <button type="button" className="shelf-suggestion" onClick={onOpenSuggestion}>
              {`suggested group: ${suggestion.title}`}
            </button>
          )}
        </div>
        {!selecting && (
          <Menu position="bottom-end" withinPortal withInitialFocusPlaceholder={false} transitionProps={{ duration: 0 }}>
            <Menu.Target>
              <ActionIcon
                variant="subtle"
                className="shelf-menu"
                aria-label={`Actions for ${item.product.name}`}
                onPointerDown={(event) => event.stopPropagation()}
              >
                <KebabIcon />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown className="bin-member-menu">
              <Menu.Item onClick={onAdd}>Add to a group…</Menu.Item>
              <Menu.Item onClick={onStartGroup}>Start a group with this</Menu.Item>
              <Menu.Item aria-expanded={selected} aria-controls={controlsId} onClick={onSelect}>
                {selected ? 'Hide instances' : 'View instances'}
              </Menu.Item>
            </Menu.Dropdown>
          </Menu>
        )}
      </div>
      {selected && children != null && (
        <div id={controlsId} className="shelf-details">
          {children}
        </div>
      )}
    </article>
  );
});

ItemRow.displayName = 'ItemRow';

import { Badge } from '@mantine/core';
import type { ExternalSource } from '../../types';

export const DATABASE_NAMES: Record<ExternalSource, string> = {
  openfoodfacts: 'Open Food Facts',
  openproductsfacts: 'Open Products Facts',
  openbeautyfacts: 'Open Beauty Facts',
  openpetfoodfacts: 'Open Pet Food Facts',
};

export interface ProvenanceBadgeProps {
  externalSource?: ExternalSource;
}

export const ProvenanceBadge = ({ externalSource }: ProvenanceBadgeProps) => {
  if (!externalSource || !(externalSource in DATABASE_NAMES)) {
    return null;
  }

  const name = DATABASE_NAMES[externalSource];

  return (
    <Badge
      size="xs"
      variant="light"
      aria-label={`Product data from ${name}`}
    >
      {name}
    </Badge>
  );
};

import { Badge } from '@mantine/core';
import type { ExternalSource } from '../../types';
import { DATABASE_NAMES } from './databaseNames';

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

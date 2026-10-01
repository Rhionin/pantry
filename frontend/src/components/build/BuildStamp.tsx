import { useEffect, useState } from 'react';
import { Text } from '@mantine/core';
import { getBuildInfo } from '../../api/client';
import type { BuildInfo } from '../../types';
import { formatBuildLabel } from '../../utils/buildLabel';

// BuildStamp shows which build is running, in type small enough to stay out
// of the scan, inventory, and shopping workflows. The subject and timestamp
// make a freshly deployed change recognizable; the commit matches the
// container tag.
export function BuildStamp() {
  const [info, setInfo] = useState<BuildInfo | null>(null);

  useEffect(() => {
    let cancelled = false;
    getBuildInfo()
      .then((buildInfo) => {
        if (!cancelled) setInfo(buildInfo);
      })
      .catch(() => {
        // A missing build identity should not interrupt the rest of the app.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (info === null) return null;
  const label = formatBuildLabel(info);
  if (label === null) return null;

  return (
    <Text
      component="div"
      size="xs"
      c="dimmed"
      role="note"
      aria-label={label.accessibleName}
      title={label.title}
      ta="right"
      style={{ width: '100%', minWidth: 0, lineHeight: 1.3, overflowWrap: 'anywhere' }}
    >
      {label.subject !== '' ? <span style={{ display: 'block' }}>{label.subject}</span> : null}
      {label.detail !== '' ? (
        <span style={{ display: 'block', fontFamily: 'var(--mono)' }}>{label.detail}</span>
      ) : null}
    </Text>
  );
}

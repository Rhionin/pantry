import { useEffect, useState } from 'react';
import { Text } from '@mantine/core';
import { getBuildInfo } from '../../api/client';
import { formatBuildLabel } from '../../utils/buildLabel';

// BuildStamp shows which build is running, in type small enough to stay out
// of the scan, inventory, and shopping workflows. The commit matches the
// container tag an operator just pulled.
export function BuildStamp() {
  const [commit, setCommit] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    getBuildInfo()
      .then((info) => {
        if (!cancelled) setCommit(info.commit);
      })
      .catch(() => {
        // A missing build identity should not interrupt the rest of the app.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (commit === null) return null;
  const label = formatBuildLabel(commit);
  if (label === null) return null;

  return (
    <Text
      size="xs"
      c="dimmed"
      ff="monospace"
      role="note"
      aria-label={label.accessibleName}
      title={commit}
      ta="right"
      style={{ maxWidth: '100%', lineHeight: 1.2, overflowWrap: 'anywhere' }}
    >
      {label.text}
    </Text>
  );
}

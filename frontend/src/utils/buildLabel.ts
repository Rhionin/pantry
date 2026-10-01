// Quiet copy for the running build. The commit is the image tag suffix
// (ghcr.io/rhionin/pantry:<commit>), so the label keeps it intact.
export interface BuildLabel {
  text: string;
  accessibleName: string;
}

// formatBuildLabel turns a commit identity into footer copy. An empty commit
// is not a build identity, so callers render nothing.
export function formatBuildLabel(commit: string): BuildLabel | null {
  if (commit === '') return null;
  const text = `build ${commit}`;
  return { text, accessibleName: text };
}

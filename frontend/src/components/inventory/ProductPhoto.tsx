import { useState } from 'react';

function PackageIcon() {
  return (
    <svg viewBox="0 0 24 24" width="22" height="22" aria-hidden="true">
      <path
        fill="currentColor"
        d="M12 2.5 3.5 7v10L12 21.5 20.5 17V7L12 2.5zm0 2.3 6.2 3.2L12 11.2 5.8 7.9 12 4.8zM5.5 9.4l5.5 2.9v6.4l-5.5-2.8V9.4zm13 0v6.5l-5.5 2.8v-6.4l5.5-2.9z"
      />
    </svg>
  );
}

export function ProductPhoto({
  src,
  name,
  stacked = false,
}: {
  src?: string;
  name: string;
  stacked?: boolean;
}) {
  const [failed, setFailed] = useState(false);
  const show = Boolean(src) && !failed;
  return (
    <span className={stacked ? 'shelf-photo is-stack' : 'shelf-photo'}>
      {show ? (
        <img src={src} alt={name} loading="lazy" onError={() => setFailed(true)} />
      ) : (
        <span className="shelf-photo-fallback" role="img" aria-label={`No photo of ${name}`}>
          <PackageIcon />
        </span>
      )}
    </span>
  );
}

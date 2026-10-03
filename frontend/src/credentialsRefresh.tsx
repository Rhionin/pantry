import { createContext, useContext } from 'react';

// Bumps after Kroger credentials are saved or cleared so the shopping page
// can reload connection state while the dialog stays open.
export const CredentialsRevisionContext = createContext(0);

export function useCredentialsRevision() {
  return useContext(CredentialsRevisionContext);
}

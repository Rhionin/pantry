import { createContext, useContext } from 'react';

export interface HouseholdSessionValue {
  required: boolean;
  logout: () => void;
}

export const HouseholdSessionContext = createContext<HouseholdSessionValue>({
  required: false,
  logout: () => {},
});

export const useHouseholdSession = () => useContext(HouseholdSessionContext);

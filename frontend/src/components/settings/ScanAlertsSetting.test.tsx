import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { ScanAlertsSetting } from './ScanAlertsSetting';
import { SCAN_ALERTS_ENABLED_KEY } from '../queue/scanAlertPreference';

class FakeNotification {
  static permission: NotificationPermission = 'default';
  static requestPermission = vi.fn(() => Promise.resolve(FakeNotification.permission));
}

const renderSetting = () => render(
  <MantineProvider>
    <ScanAlertsSetting />
  </MantineProvider>,
);

describe('ScanAlertsSetting', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it('explains that scan alerts are blocked when the browser denied permission', () => {
    FakeNotification.permission = 'denied';
    FakeNotification.requestPermission = vi.fn();
    vi.stubGlobal('Notification', FakeNotification);

    renderSetting();

    const toggle = screen.getByRole('switch', { name: 'Scan alerts' });
    expect(toggle).toBeChecked();
    expect(toggle).toHaveAccessibleDescription(/Failed scans and unrecognized products alert right away/);
    expect(screen.getByRole('status')).toHaveTextContent(
      "Scan alerts are blocked. Re-allow notifications for this site in the browser's site settings.",
    );
    expect(FakeNotification.requestPermission).not.toHaveBeenCalled();
  });

  it('keeps the denied note when scan alerts stay on and does not ask the browser again', () => {
    FakeNotification.permission = 'denied';
    FakeNotification.requestPermission = vi.fn();
    vi.stubGlobal('Notification', FakeNotification);
    localStorage.setItem(SCAN_ALERTS_ENABLED_KEY, 'off');

    renderSetting();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('switch', { name: 'Scan alerts' }));

    expect(screen.getByRole('switch', { name: 'Scan alerts' })).toBeChecked();
    expect(screen.getByRole('status')).toHaveTextContent(/browser's site settings/);
    expect(FakeNotification.requestPermission).not.toHaveBeenCalled();
    expect(FakeNotification.permission).toBe('denied');
  });

  it('requests permission when scan alerts are switched on and the browser has not decided', async () => {
    FakeNotification.permission = 'default';
    FakeNotification.requestPermission = vi.fn(() => {
      FakeNotification.permission = 'granted';
      return Promise.resolve<NotificationPermission>('granted');
    });
    vi.stubGlobal('Notification', FakeNotification);
    localStorage.setItem(SCAN_ALERTS_ENABLED_KEY, 'off');

    renderSetting();
    fireEvent.click(screen.getByRole('switch', { name: 'Scan alerts' }));

    await waitFor(() => expect(FakeNotification.requestPermission).toHaveBeenCalledOnce());
    expect(screen.getByRole('switch', { name: 'Scan alerts' })).toBeChecked();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('explains when the browser cannot show notifications from an open tab', () => {
    vi.stubGlobal('Notification', undefined);
    renderSetting();
    expect(screen.getByRole('status')).toHaveTextContent(
      'This browser cannot show notifications from an open tab.',
    );
    expect(screen.getByRole('switch', { name: 'Scan alerts' })).toBeChecked();
  });
});

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MantineProvider } from '@mantine/core';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { LoginPage } from './LoginPage';

const renderLogin = (onSignedIn: () => void = () => undefined) => render(
  <MantineProvider>
    <LoginPage onSignedIn={onSignedIn} />
  </MantineProvider>,
);

describe('LoginPage', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('asks for the household username and password', () => {
    renderLogin();
    expect(screen.getByRole('heading', { name: 'Pantry' })).toBeVisible();
    expect(screen.getByRole('form', { name: 'Sign in' })).toBeVisible();
    expect(screen.getByLabelText(/Username/)).toBeVisible();
    expect(screen.getByLabelText(/Password/)).toBeVisible();
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeEnabled();
    expect(document.querySelector('.login-logo')).toHaveAttribute('src', '/brand/logo.png');
  });

  it('signs in with the username and password that were entered', async () => {
    const onSignedIn = vi.fn();
    const fetchMock = vi.fn(() => Promise.resolve(new Response(JSON.stringify({ username: 'pantry' }), { status: 200 })));
    vi.stubGlobal('fetch', fetchMock);
    renderLogin(onSignedIn);

    fireEvent.change(screen.getByLabelText(/Username/), { target: { value: 'pantry' } });
    fireEvent.change(screen.getByLabelText(/Password/), { target: { value: 'correct-horse-battery' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    await waitFor(() => expect(onSignedIn).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith('/api/login', expect.objectContaining({
      method: 'POST',
      credentials: 'same-origin',
      body: JSON.stringify({ username: 'pantry', password: 'correct-horse-battery' }),
    }));
  });

  it('shows the server error and stays on the form', async () => {
    const onSignedIn = vi.fn();
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response(
      JSON.stringify({ error: 'Too many sign-in attempts. Try again later.' }),
      { status: 429, headers: { 'Content-Type': 'application/json' } },
    ))));
    renderLogin(onSignedIn);

    fireEvent.change(screen.getByLabelText(/Username/), { target: { value: 'pantry' } });
    fireEvent.change(screen.getByLabelText(/Password/), { target: { value: 'nope' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Too many sign-in attempts. Try again later.');
    expect(onSignedIn).not.toHaveBeenCalled();
    expect(screen.getByRole('form', { name: 'Sign in' })).toBeVisible();
  });
});

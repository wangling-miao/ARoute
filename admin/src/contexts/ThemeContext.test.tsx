import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ThemeProvider, useTheme } from './ThemeContext';

function ThemeConsumer() {
  const { theme, toggleTheme, setTheme } = useTheme();
  return (
    <div>
      <span data-testid="current-theme">{theme}</span>
      <button type="button" data-testid="toggle" onClick={toggleTheme}>Toggle</button>
      <button type="button" data-testid="set-dark" onClick={() => setTheme('dark')}>Set Dark</button>
      <button type="button" data-testid="set-light" onClick={() => setTheme('light')}>Set Light</button>
    </div>
  );
}

function renderWithProvider() {
  return render(
    <ThemeProvider>
      <ThemeConsumer />
    </ThemeProvider>,
  );
}

describe('ThemeContext', () => {
  beforeEach(() => {
    localStorage.clear();
    document.body.removeAttribute('arco-theme');
    document.documentElement.removeAttribute('data-theme');
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: false }));
  });

  it('defaults to light when no stored preference and system prefers light', () => {
    renderWithProvider();
    expect(screen.getByTestId('current-theme')).toHaveTextContent('light');
  });

  it('defaults to dark when system prefers dark and no stored preference', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: true }));
    renderWithProvider();
    expect(screen.getByTestId('current-theme')).toHaveTextContent('dark');
  });

  it('reads stored theme from localStorage', () => {
    localStorage.setItem('aroute_theme', 'dark');
    renderWithProvider();
    expect(screen.getByTestId('current-theme')).toHaveTextContent('dark');
  });

  it('toggles between light and dark', async () => {
    renderWithProvider();
    expect(screen.getByTestId('current-theme')).toHaveTextContent('light');

    await userEvent.click(screen.getByTestId('toggle'));
    expect(screen.getByTestId('current-theme')).toHaveTextContent('dark');

    await userEvent.click(screen.getByTestId('toggle'));
    expect(screen.getByTestId('current-theme')).toHaveTextContent('light');
  });

  it('persists theme to localStorage on change', async () => {
    renderWithProvider();
    await userEvent.click(screen.getByTestId('set-dark'));
    expect(localStorage.getItem('aroute_theme')).toBe('dark');
  });

  it('sets arco-theme attribute on body for dark mode', async () => {
    renderWithProvider();
    await userEvent.click(screen.getByTestId('set-dark'));
    expect(document.body.getAttribute('arco-theme')).toBe('dark');
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
  });

  it('removes arco-theme attribute from body for light mode', async () => {
    localStorage.setItem('aroute_theme', 'dark');
    renderWithProvider();
    expect(document.body.getAttribute('arco-theme')).toBe('dark');

    await userEvent.click(screen.getByTestId('set-light'));
    expect(document.body.getAttribute('arco-theme')).toBeNull();
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
  });

  it('throws when useTheme is used outside ThemeProvider', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
    expect(() => render(<ThemeConsumer />)).toThrow('useTheme must be used within a ThemeProvider');
    spy.mockRestore();
  });
});

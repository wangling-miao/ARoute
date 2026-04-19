import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import Dashboard from './Dashboard';
import type { DashboardStats } from '@/types';

const mockDashboardGetStats = vi.fn();
vi.mock('@/api/endpoints', () => ({
  dashboard: { getStats: () => mockDashboardGetStats() },
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) => {
      if (opts) return `${key}:${JSON.stringify(opts)}`;
      return key;
    },
  }),
}));

vi.mock('./Dashboard.module.css', () => ({
  default: new Proxy({}, { get: (_, prop) => prop }),
}));

const MOCK_STATS: DashboardStats = {
  content_counts: { posts: 12, pages: 5 },
  recent_activity: [
    { id: '1', action: 'create', resource_type: 'post', resource_id: 'abc123456', user_id: 'u1', created_at: new Date().toISOString() },
  ],
  system_status: { database: 'healthy' as const, plugin_count: 7, cache_hit_ratio: 0.85 },
};

describe('Dashboard page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows loading skeleton initially', () => {
    mockDashboardGetStats.mockReturnValue(new Promise(() => {}));
    render(<Dashboard />);
    expect(document.querySelector('.skeleton')).toBeInTheDocument();
  });

  it('renders content counts after loading', async () => {
    mockDashboardGetStats.mockResolvedValueOnce(MOCK_STATS);
    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getByText('12')).toBeInTheDocument();
      expect(screen.getByText('5')).toBeInTheDocument();
    });
  });

  it('renders recent activity items', async () => {
    mockDashboardGetStats.mockResolvedValueOnce(MOCK_STATS);
    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getByText(/dashboard\.action_create.*post/i)).toBeInTheDocument();
    });
  });

  it('renders system status section', async () => {
    mockDashboardGetStats.mockResolvedValueOnce(MOCK_STATS);
    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getByText('7')).toBeInTheDocument();
      expect(screen.getByText('85%')).toBeInTheDocument();
    });
  });

  it('shows error state with retry button on fetch failure', async () => {
    mockDashboardGetStats.mockRejectedValueOnce(new Error('fail'));
    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getByText('common.error_occurred')).toBeInTheDocument();
      expect(screen.getByText('common.try_again')).toBeInTheDocument();
    });
  });

  it('shows empty activity when no recent_activity', async () => {
    mockDashboardGetStats.mockResolvedValueOnce({
      ...MOCK_STATS,
      recent_activity: [],
    });
    render(<Dashboard />);

    await waitFor(() => {
      expect(screen.getByText('dashboard.no_activity')).toBeInTheDocument();
    });
  });
});

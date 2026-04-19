import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import ContentList from './ContentList';
import type { ContentItem, ContentType, PaginatedResponse } from '@/types';

const mockContentList = vi.fn();
const mockContentTypesGet = vi.fn();
const mockContentDelete = vi.fn();

vi.mock('@/api/endpoints', () => ({
  content: {
    list: (...args: unknown[]) => mockContentList(...args),
    delete: (...args: unknown[]) => mockContentDelete(...args),
  },
  contentTypes: {
    get: (...args: unknown[]) => mockContentTypesGet(...args),
  },
}));

vi.mock('@/components/Toast', () => ({
  showSuccess: vi.fn(),
  showError: vi.fn(),
}));

const mockConfirmFn = vi.fn();
vi.mock('@/components/ConfirmDialog', () => ({
  confirm: (opts: { onConfirm: () => void }) => mockConfirmFn(opts),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) => {
      if (opts) return `${key}:${JSON.stringify(opts)}`;
      return key;
    },
  }),
}));

vi.mock('./ContentList.module.css', () => ({
  default: new Proxy({}, { get: (_, prop) => prop }),
}));

const MOCK_CT: ContentType = {
  name: 'posts',
  display_name: 'Posts',
  description: 'Blog posts',
  table_name: 'content_posts',
  fields: [{ name: 'title', display_name: 'Title', type: 'text', required: true, unique: false }],
};

function makeItem(overrides: Partial<ContentItem> = {}): ContentItem {
  return {
    id: 'item-1',
    content_type: 'posts',
    data: { title: 'My First Post' },
    status: 'published',
    author_id: 'u1',
    created_at: '2025-01-15T10:00:00Z',
    updated_at: '2025-01-15T12:00:00Z',
    ...overrides,
  };
}

function makePaginated(items: ContentItem[], total = items.length): PaginatedResponse<ContentItem> {
  return { data: items, meta: { total: total, page: 1, per_page: 20, total_pages: 1 } };
}

function renderContentList() {
  return render(
    <MemoryRouter initialEntries={['/admin/content/posts']}>
      <Routes>
        <Route path="/admin/content/:contentType" element={<ContentList />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('ContentList page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockContentTypesGet.mockResolvedValue(MOCK_CT);
  });

  it('shows loading state then content table', async () => {
    mockContentList.mockResolvedValueOnce(makePaginated([makeItem()]));

    renderContentList();

    await waitFor(() => {
      expect(screen.getByText('My First Post')).toBeInTheDocument();
    });
  });

  it('shows empty state when no items', async () => {
    mockContentList.mockResolvedValueOnce(makePaginated([]));

    renderContentList();

    await waitFor(() => {
      expect(screen.getByText('content.no_content')).toBeInTheDocument();
    });
  });

  it('displays content type display name in breadcrumb', async () => {
    mockContentList.mockResolvedValueOnce(makePaginated([]));

    renderContentList();

    await waitFor(() => {
      expect(screen.getByText('Posts')).toBeInTheDocument();
    });
  });

  it('triggers delete confirmation when delete button is clicked', async () => {
    mockContentList.mockResolvedValueOnce(makePaginated([makeItem()]));

    renderContentList();

    await waitFor(() => {
      expect(screen.getByText('My First Post')).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByTitle('common.delete');
    await userEvent.click(deleteButtons[0]);

    expect(mockConfirmFn).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'common.delete',
        danger: true,
      }),
    );
  });

  it('executes delete on confirm callback', async () => {
    mockContentList.mockResolvedValue(makePaginated([makeItem()]));
    mockContentDelete.mockResolvedValueOnce(undefined);

    renderContentList();

    await waitFor(() => {
      expect(screen.getByText('My First Post')).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByTitle('common.delete');
    await userEvent.click(deleteButtons[0]);

    const confirmOpts = mockConfirmFn.mock.calls[0][0];
    await confirmOpts.onConfirm();

    expect(mockContentDelete).toHaveBeenCalledWith('posts', 'item-1');
  });

  it('passes list params to API on initial load', async () => {
    mockContentList.mockResolvedValueOnce(makePaginated([]));

    renderContentList();

    await waitFor(() => {
      expect(mockContentList).toHaveBeenCalledWith(
        'posts',
        expect.objectContaining({ page: 1, per_page: 20, sort: 'created_at', order: 'desc' }),
      );
    });
  });
});

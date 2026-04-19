import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import ApiTokens from './ApiTokens';
import type { ApiToken } from '@/types';

const { mockTokensList, mockTokensCreate, mockTokensRevoke, mockConfirmFn, tokensT } = vi.hoisted(() => {
  const map: Record<string, string> = {
    'api_tokens.title': 'API Tokens',
    'api_tokens.create': 'Create Token',
    'api_tokens.token_name': 'Token Name',
    'api_tokens.preview': 'Preview',
    'api_tokens.created': 'Created',
    'api_tokens.last_used': 'Last Used',
    'api_tokens.expires_at': 'Expires',
    'api_tokens.never': 'Never',
    'api_tokens.revoke': 'Revoke',
    'api_tokens.revoke_confirm': 'Are you sure?',
    'api_tokens.revoked_success': 'Revoked',
    'api_tokens.created_success': 'Token created',
    'api_tokens.no_tokens': 'No tokens yet',
    'api_tokens.token_created_warning': 'Copy now',
    'api_tokens.copy_token': 'Copy',
    'common.cancel': 'Cancel',
    'common.create': 'Create',
    'common.close': 'Close',
    'common.error_occurred': 'Error',
    'common.try_again': 'Try Again',
    'common.actions': 'Actions',
    'common.copied': 'Copied',
  };
  return {
    mockTokensList: vi.fn(),
    mockTokensCreate: vi.fn(),
    mockTokensRevoke: vi.fn(),
    mockConfirmFn: vi.fn(),
    tokensT: (key: string) => map[key] || key,
  };
});

vi.mock('@arco-design/web-react', async () => {
  const { createElement: h, forwardRef } = await import('react');
  let cachedForm: Record<string, unknown> | null = null;
  const formStore: Record<string, unknown> = {};

  const FormComponent = (props: Record<string, unknown>) => h('form', null, props.children as React.ReactNode);
  const Form = Object.assign(FormComponent, {
    useForm: () => {
      if (!cachedForm) {
        cachedForm = {
          setFieldsValue: (v: Record<string, unknown>) => Object.assign(formStore, v),
          validate: () => Promise.resolve({ ...formStore }),
          resetFields: () => { for (const k of Object.keys(formStore)) delete formStore[k]; },
        };
      }
      return [cachedForm];
    },
    Item: (props: Record<string, unknown>) =>
      h('div', null, props.label ? h('label', null, props.label as string) : null, props.children as React.ReactNode),
  });

  return {
    Button: forwardRef((props: Record<string, unknown>, ref: React.Ref<HTMLButtonElement>) =>
      h('button', {
        ref,
        onClick: props.onClick as React.MouseEventHandler,
        disabled: props.disabled as boolean || props.loading as boolean,
        className: [
          (props.type as string) === 'primary' ? 'primary' : '',
          (props.status as string) === 'danger' ? 'danger' : '',
        ].filter(Boolean).join(' '),
      }, props.icon as React.ReactNode, props.children as React.ReactNode),
    ),
    Spin: () => h('div', { className: 'arco-spin' }),
    Typography: { Title: (props: Record<string, unknown>) => h('h5', null, props.children as React.ReactNode) },
    Form,
    Input: forwardRef((props: Record<string, unknown>, ref: React.Ref<HTMLInputElement>) =>
      h('input', { ref, ...props } as Record<string, unknown>),
    ),
    Table: (props: Record<string, unknown>) => {
      const columns = props.columns as Array<Record<string, unknown>>;
      const data = props.data as Array<Record<string, unknown>>;
      if (!data || data.length === 0) return null;
      return h('table', null,
        h('tbody', null,
          data.map((row, i) =>
            h('tr', { key: (row.id as string) || i },
              columns?.map((col) =>
                h('td', { key: col.key as string },
                  typeof col.render === 'function'
                    ? col.render(row[col.dataIndex as string], row)
                    : row[col.dataIndex as string],
                ),
              ),
            ),
          ),
        ),
      );
    },
    Modal: (props: Record<string, unknown>) => {
      if (!props.visible) return null;
      return h('div', { className: 'arco-modal', role: 'dialog' },
        h('div', null, props.title as React.ReactNode),
        h('div', null, props.children as React.ReactNode),
        h('div', null, props.footer as React.ReactNode),
      );
    },
    Tag: (props: Record<string, unknown>) => h('span', props, props.children as React.ReactNode),
    DatePicker: Object.assign(
      (props: Record<string, unknown>) => h('input', { type: 'text', ...props }),
      { RangePicker: (props: Record<string, unknown>) => h('input', { type: 'text', ...props }) },
    ),
  };
});

vi.mock('@/api/endpoints', () => ({
  apiTokens: {
    list: () => mockTokensList(),
    create: (data: unknown) => mockTokensCreate(data),
    revoke: (id: string) => mockTokensRevoke(id),
  },
}));

vi.mock('@/api/client', () => ({
  ApiError: class ApiError extends Error {
    code: string;
    constructor(code: string, message: string) { super(message); this.code = code; }
  },
}));

vi.mock('@/components/Toast', () => ({
  showSuccess: vi.fn(),
  showError: vi.fn(),
  showInfo: vi.fn(),
}));

vi.mock('@/components/ConfirmDialog', () => ({
  confirm: (opts: Record<string, unknown>) => mockConfirmFn(opts),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: tokensT }),
}));

vi.mock('./ApiTokens.module.css', () => ({
  default: new Proxy({}, { get: (_, prop) => prop }),
}));

const MOCK_TOKENS: ApiToken[] = [
  {
    id: 'tok-1',
    name: 'CI/CD',
    token_preview: 'aroute_abc...xyz',
    created_at: '2025-01-10T08:00:00Z',
    last_used_at: '2025-01-15T08:00:00Z',
    expires_at: undefined,
  },
];

function renderApiTokens() {
  return render(
    <MemoryRouter>
      <ApiTokens />
    </MemoryRouter>,
  );
}

describe('ApiTokens page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('shows loading spinner initially', () => {
    mockTokensList.mockReturnValue(new Promise(() => {}));
    renderApiTokens();
    expect(document.querySelector('.arco-spin')).toBeInTheDocument();
  });

  it('renders token list after loading', async () => {
    mockTokensList.mockResolvedValueOnce(MOCK_TOKENS);
    renderApiTokens();
    await waitFor(() => {
      expect(screen.getByText('CI/CD')).toBeInTheDocument();
      expect(screen.getByText('aroute_abc...xyz')).toBeInTheDocument();
    });
  });

  it('shows empty state when no tokens', async () => {
    mockTokensList.mockResolvedValueOnce([]);
    renderApiTokens();
    await waitFor(() => {
      expect(screen.getByText('No tokens yet')).toBeInTheDocument();
    });
  });

  it('opens create modal when create button is clicked', async () => {
    mockTokensList.mockResolvedValueOnce(MOCK_TOKENS);
    renderApiTokens();
    await waitFor(() => {
      expect(screen.getByText('CI/CD')).toBeInTheDocument();
    });
    const createBtn = document.querySelector('button[class*="primary"]');
    expect(createBtn).toBeTruthy();
    await userEvent.click(createBtn!);
    await waitFor(() => {
      expect(screen.getByText('Token Name')).toBeInTheDocument();
    });
  });

  it('shows token value after creation', async () => {
    mockTokensList
      .mockResolvedValueOnce(MOCK_TOKENS)
      .mockResolvedValueOnce(MOCK_TOKENS);
    mockTokensCreate.mockResolvedValueOnce({ token: 'aroute_secret_new_token_123' });
    renderApiTokens();
    await waitFor(() => {
      expect(screen.getByText('CI/CD')).toBeInTheDocument();
    });
    const createBtn = document.querySelector('button[class*="primary"]');
    await userEvent.click(createBtn!);
    await waitFor(() => {
      expect(screen.getByText('Token Name')).toBeInTheDocument();
    });
    const nameInput = screen.getByPlaceholderText('e.g. CI/CD Pipeline');
    await userEvent.type(nameInput, 'My New Token');
    const allCreateBtns = screen.getAllByText('Create');
    await userEvent.click(allCreateBtns[allCreateBtns.length - 1]);
    await waitFor(() => {
      expect(screen.getByText('aroute_secret_new_token_123')).toBeInTheDocument();
    });
  });

  it('triggers revoke confirmation when revoke icon is clicked', async () => {
    mockTokensList.mockResolvedValueOnce(MOCK_TOKENS);
    renderApiTokens();
    await waitFor(() => {
      expect(screen.getByText('CI/CD')).toBeInTheDocument();
    });
    const dangerButtons = document.querySelectorAll('button[class*="danger"]');
    expect(dangerButtons.length).toBeGreaterThan(0);
    await userEvent.click(dangerButtons[0]);
    expect(mockConfirmFn).toHaveBeenCalled();
  });

  it('shows error state on fetch failure', async () => {
    mockTokensList.mockRejectedValueOnce(new Error('fail'));
    renderApiTokens();
    await waitFor(() => {
      expect(screen.getByText('Error')).toBeInTheDocument();
      expect(screen.getByText('Try Again')).toBeInTheDocument();
    });
  });
});

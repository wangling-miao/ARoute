import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import Settings from './Settings';
import type { Settings as SettingsType } from '@/types';

const { mockSettingsGet, mockSettingsUpdate, settingsT } = vi.hoisted(() => {
  const map: Record<string, string> = {
    'settings.title': 'Settings',
    'settings.general': 'General',
    'settings.email': 'Email',
    'settings.site_name': 'Site Name',
    'settings.site_url': 'Site URL',
    'settings.language': 'Language',
    'settings.timezone': 'Timezone',
    'settings.smtp_host': 'SMTP Host',
    'settings.smtp_port': 'SMTP Port',
    'settings.smtp_username': 'SMTP Username',
    'settings.sender_email': 'Sender Email',
    'settings.save_success': 'Settings saved',
    'common.save': 'Save',
    'common.error_occurred': 'An error occurred',
  };
  return {
    mockSettingsGet: vi.fn(),
    mockSettingsUpdate: vi.fn(),
    settingsT: (key: string) => map[key] || key,
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
    InputNumber: (props: Record<string, unknown>) => h('input', { type: 'number', ...props }),
    Select: Object.assign(
      (props: Record<string, unknown>) => h('select', props, props.children as React.ReactNode),
      { Option: (props: Record<string, unknown>) => h('option', { value: props.value }, props.children as React.ReactNode) },
    ),
  };
});

vi.mock('@/api/endpoints', () => ({
  settings: {
    get: () => mockSettingsGet(),
    update: (data: unknown) => mockSettingsUpdate(data),
  },
}));

vi.mock('@/api/client', () => ({
  ApiError: class ApiError extends Error {
    code: string;
    constructor(code: string, message: string) { super(message); this.code = code; }
  },
}));

vi.mock('@/components/Toast', () => ({
  showError: vi.fn(),
  showSuccess: vi.fn(),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: settingsT }),
}));

vi.mock('./Settings.module.css', () => ({
  default: new Proxy({}, { get: (_, prop) => prop }),
}));

const MOCK_SETTINGS: SettingsType = {
  site_name: 'My Site',
  site_url: 'https://example.com',
  language: 'en',
  timezone: 'UTC',
  smtp_host: 'smtp.example.com',
  smtp_port: 587,
  smtp_username: 'user',
  sender_email: 'noreply@example.com',
};

function renderSettings() {
  return render(
    <MemoryRouter>
      <Settings />
    </MemoryRouter>,
  );
}

describe('Settings page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('shows loading spinner initially', () => {
    mockSettingsGet.mockReturnValue(new Promise(() => {}));
    renderSettings();
    expect(document.querySelector('.arco-spin')).toBeInTheDocument();
  });

  it('renders settings form after loading', async () => {
    mockSettingsGet.mockResolvedValueOnce(MOCK_SETTINGS);
    renderSettings();
    await waitFor(() => {
      expect(screen.getByText('Settings')).toBeInTheDocument();
    });
  });

  it('calls settings API on mount', async () => {
    mockSettingsGet.mockResolvedValueOnce(MOCK_SETTINGS);
    renderSettings();
    await waitFor(() => {
      expect(mockSettingsGet).toHaveBeenCalledTimes(1);
    });
  });

  it('shows general section by default', async () => {
    mockSettingsGet.mockResolvedValueOnce(MOCK_SETTINGS);
    renderSettings();
    await waitFor(() => {
      expect(screen.getByText('Site Name')).toBeInTheDocument();
      expect(screen.getByText('Site URL')).toBeInTheDocument();
    });
  });

  it('switches to email section when email nav is clicked', async () => {
    mockSettingsGet.mockResolvedValueOnce(MOCK_SETTINGS);
    const { container } = renderSettings();
    await waitFor(() => {
      expect(screen.getByText('Site Name')).toBeInTheDocument();
    });
    const navButtons = container.querySelectorAll('button');
    const emailBtn = Array.from(navButtons).find(b => b.textContent?.includes('Email'));
    expect(emailBtn).toBeTruthy();
    await userEvent.click(emailBtn!);
    await waitFor(() => {
      expect(screen.getByText('SMTP Host')).toBeInTheDocument();
    });
  });

  it('calls settings update on save button click', async () => {
    mockSettingsGet.mockResolvedValueOnce(MOCK_SETTINGS);
    mockSettingsUpdate.mockResolvedValueOnce(MOCK_SETTINGS);
    renderSettings();
    await waitFor(() => {
      expect(screen.getByText('Save')).toBeInTheDocument();
    });
    await userEvent.click(screen.getByText('Save'));
    await waitFor(() => {
      expect(mockSettingsUpdate).toHaveBeenCalled();
    });
  });

  it('hides spinner after fetch failure', async () => {
    mockSettingsGet.mockRejectedValueOnce(new Error('fail'));
    renderSettings();
    await waitFor(() => {
      expect(document.querySelector('.arco-spin')).not.toBeInTheDocument();
    });
  });
});

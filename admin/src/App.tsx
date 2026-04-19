import { Component, type ReactNode, type ErrorInfo } from 'react';
import { RouterProvider } from 'react-router-dom';
import { ConfigProvider } from '@arco-design/web-react';
import enUS from '@arco-design/web-react/es/locale/en-US';
import zhCN from '@arco-design/web-react/es/locale/zh-CN';
import { I18nextProvider } from 'react-i18next';
import { ThemeProvider, useTheme } from '@/contexts/ThemeContext';
import { router } from '@/router';
import i18n from '@/i18n';
import '@arco-design/web-react/dist/css/arco.css';
import '@/styles/global.css';

function getArcoLocale() {
  const lang = i18n.language?.startsWith('zh') ? 'zh' : 'en';
  return lang === 'zh' ? zhCN : enUS;
}

function ThemedApp() {
  useTheme();

  return (
    <ConfigProvider
      locale={getArcoLocale()}
      getPopupContainer={() => document.body}
    >
      <RouterProvider router={router} />
    </ConfigProvider>
  );
}

interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
}

class GlobalErrorBoundary extends Component<
  { children: ReactNode },
  ErrorBoundaryState
> {
  state: ErrorBoundaryState = { hasError: false, error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Global error boundary caught:', error, info);
  }

  render() {
    if (this.state.hasError) {
      return (
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            minHeight: '100vh',
            padding: '2rem',
            fontFamily: "'Outfit', system-ui, sans-serif",
            background: '#eef2f7',
            color: '#1a1f36',
          }}
        >
          <div style={{
            background: 'rgba(255,255,255,0.6)',
            backdropFilter: 'blur(24px)',
            border: '1px solid rgba(0,0,0,0.06)',
            borderRadius: 28,
            padding: '3rem 2.5rem',
            textAlign: 'center',
            maxWidth: 420,
          }}>
            <h1 style={{ fontSize: '1.5rem', marginBottom: '0.75rem', fontWeight: 700, letterSpacing: '-0.02em' }}>
              Something went wrong
            </h1>
            <p style={{ color: '#7c82a0', marginBottom: '1.5rem', fontSize: '0.9375rem', lineHeight: 1.6 }}>
              {this.state.error?.message || 'An unexpected error occurred'}
            </p>
            <button
              type="button"
              onClick={() => {
                this.setState({ hasError: false, error: null });
                window.location.href = '/admin/';
              }}
              style={{
                padding: '0.625rem 1.75rem',
                borderRadius: 14,
                border: 'none',
                background: 'linear-gradient(135deg, #4facfe 0%, #00c6fb 100%)',
                color: 'white',
                cursor: 'pointer',
                fontSize: '0.875rem',
                fontWeight: 600,
                fontFamily: "'Outfit', system-ui, sans-serif",
                boxShadow: '0 4px 16px rgba(79, 172, 254, 0.3)',
                transition: 'all 0.2s ease',
              }}
            >
              Return to Dashboard
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

export default function App() {
  return (
    <GlobalErrorBoundary>
      <I18nextProvider i18n={i18n}>
        <ThemeProvider>
          <ThemedApp />
        </ThemeProvider>
      </I18nextProvider>
    </GlobalErrorBoundary>
  );
}

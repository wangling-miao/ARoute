import { createContext, useContext, useCallback, useState, useEffect, useRef, type ReactNode } from 'react';
import { Navigate, useLocation, useNavigate, Outlet } from 'react-router-dom';
import { auth } from '@/api/endpoints';
import { clearTokens, setOnAuthFailure } from '@/api/client';
import type { User } from '@/types';

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
}

interface AuthContextValue extends AuthState {
  login: (email: string, password: string, rememberMe?: boolean) => Promise<void>;
  logout: () => void;
  refreshSession: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({
    user: null,
    isAuthenticated: false,
    isLoading: true,
  });
  const navigate = useNavigate();
  const location = useLocation();
  const mountedRef = useRef(true);

  const handleAuthFailure = useCallback(() => {
    if (mountedRef.current) {
      setState({ user: null, isAuthenticated: false, isLoading: false });
    }
  }, []);

  useEffect(() => {
    setOnAuthFailure(handleAuthFailure);
  }, [handleAuthFailure]);

  const refreshSession = useCallback(async () => {
    try {
      const user = await auth.getCurrentUser();
      if (mountedRef.current) {
        setState({ user, isAuthenticated: true, isLoading: false });
      }
    } catch {
      if (mountedRef.current) {
        setState({ user: null, isAuthenticated: false, isLoading: false });
      }
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    refreshSession();
    return () => {
      mountedRef.current = false;
    };
  }, [refreshSession]);

  const login = useCallback(async (email: string, password: string, rememberMe?: boolean) => {
    await auth.login(email, password, rememberMe);
    const user = await auth.getCurrentUser();
    if (mountedRef.current) {
      setState({ user, isAuthenticated: true, isLoading: false });
    }
    const from = (location.state as { from?: { pathname: string } })?.from?.pathname || '/admin/';
    navigate(from, { replace: true });
  }, [navigate, location.state]);

  const logout = useCallback(() => {
    auth.logout();
    clearTokens();
    if (mountedRef.current) {
      setState({ user: null, isAuthenticated: false, isLoading: false });
    }
    navigate('/admin/login', { replace: true });
  }, [navigate]);

  return (
    <AuthContext.Provider value={{ ...state, login, logout, refreshSession }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}

export function ProtectedRoute({ children }: { children: ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth();
  const location = useLocation();

  if (isLoading) {
    return (
      <div className="page-loading">
        <div className="loading-spinner" />
      </div>
    );
  }

  if (!isAuthenticated) {
    return <Navigate to="/admin/login" state={{ from: location }} replace />;
  }

  return <>{children}</>;
}

export function GuestRoute({ children }: { children: ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth();

  if (isLoading) {
    return (
      <div className="page-loading">
        <div className="loading-spinner" />
      </div>
    );
  }

  if (isAuthenticated) {
    return <Navigate to="/admin/" replace />;
  }

  return <>{children}</>;
}

export function RouterAuthProvider() {
  return (
    <AuthProvider>
      <Outlet />
    </AuthProvider>
  );
}

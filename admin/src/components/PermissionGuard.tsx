import { type ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { usePermissions } from '@/hooks/usePermissions';

interface PermissionGuardProps {
  children: ReactNode;
  permissions: Array<{ resource: string; action: string }>;
}

export default function PermissionGuard({ children, permissions }: PermissionGuardProps) {
  const { canAny } = usePermissions();

  if (!canAny(permissions)) {
    return <Navigate to="/admin/" replace />;
  }

  return <>{children}</>;
}

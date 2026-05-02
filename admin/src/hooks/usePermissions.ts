import { useAuth } from '@/contexts/AuthContext';

export function usePermissions() {
  const { hasPermission, user } = useAuth();

  return {
    isAdmin: user?.roles?.includes('admin') ?? false,
    can: hasPermission,
    canAny: (checks: Array<{ resource: string; action: string }>) =>
      checks.some(({ resource, action }) => hasPermission(resource, action)),
  };
}

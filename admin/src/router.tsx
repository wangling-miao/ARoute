import { lazy, Suspense } from 'react';
import { createBrowserRouter, Navigate } from 'react-router-dom';
import { ProtectedRoute, GuestRoute, RouterAuthProvider } from '@/contexts/AuthContext';
import PermissionGuard from '@/components/PermissionGuard';
import Login from '@/pages/Login';
import Layout from '@/components/Layout';

const Dashboard = lazy(() => import('@/pages/Dashboard'));
const ContentList = lazy(() => import('@/pages/ContentList'));
const ContentEdit = lazy(() => import('@/pages/ContentEdit'));
const ContentTypeList = lazy(() => import('@/pages/ContentTypeList'));
const ContentTypeBuilder = lazy(() => import('@/pages/ContentTypeBuilder'));
const MenuManagement = lazy(() => import('@/pages/MenuManagement'));
const MediaLibrary = lazy(() => import('@/pages/MediaLibrary'));
const UserManagement = lazy(() => import('@/pages/UserManagement'));
const RoleManagement = lazy(() => import('@/pages/RoleManagement'));
const PluginManagement = lazy(() => import('@/pages/PluginManagement'));
const Settings = lazy(() => import('@/pages/Settings'));
const ApiTokens = lazy(() => import('@/pages/ApiTokens'));

function SuspenseFallback() {
  return (
    <div className="page-loading">
      <div className="loading-spinner" />
    </div>
  );
}

function LazyPage({ Component }: { Component: React.LazyExoticComponent<React.ComponentType> }) {
  return (
    <Suspense fallback={<SuspenseFallback />}>
      <Component />
    </Suspense>
  );
}

function PG({ children, resource, action }: { children: React.ReactNode; resource: string; action: string }) {
  return (
    <PermissionGuard permissions={[{ resource, action }]}>
      {children}
    </PermissionGuard>
  );
}

export const router = createBrowserRouter([
  {
    element: <RouterAuthProvider />,
    children: [
      {
        path: '/admin/login',
        element: (
          <GuestRoute>
            <Login />
          </GuestRoute>
        ),
      },
      {
        path: '/admin/',
        element: (
          <ProtectedRoute>
            <Layout />
          </ProtectedRoute>
        ),
        children: [
          { index: true, element: <LazyPage Component={Dashboard} /> },
          { path: 'content/:contentType', element: <PG resource="content" action="read"><LazyPage Component={ContentList} /></PG> },
          { path: 'content/:contentType/new', element: <PG resource="content" action="create"><LazyPage Component={ContentEdit} /></PG> },
          { path: 'content/:contentType/:id', element: <PG resource="content" action="update"><LazyPage Component={ContentEdit} /></PG> },
          { path: 'content-types', element: <PG resource="content_types" action="read"><LazyPage Component={ContentTypeList} /></PG> },
          { path: 'content-types/new', element: <PG resource="content_types" action="create"><LazyPage Component={ContentTypeBuilder} /></PG> },
          { path: 'content-types/:name', element: <PG resource="content_types" action="update"><LazyPage Component={ContentTypeBuilder} /></PG> },
          { path: 'menus', element: <PG resource="menus" action="read"><LazyPage Component={MenuManagement} /></PG> },
          { path: 'media', element: <PG resource="media" action="read"><LazyPage Component={MediaLibrary} /></PG> },
          { path: 'users', element: <PG resource="users" action="read"><LazyPage Component={UserManagement} /></PG> },
          { path: 'roles', element: <PG resource="roles" action="read"><LazyPage Component={RoleManagement} /></PG> },
          { path: 'plugins', element: <PG resource="plugins" action="read"><LazyPage Component={PluginManagement} /></PG> },
          { path: 'settings', element: <PG resource="settings" action="read"><LazyPage Component={Settings} /></PG> },
          { path: 'api-tokens', element: <PG resource="api_tokens" action="read"><LazyPage Component={ApiTokens} /></PG> },
        ],
      },
      {
        path: '/admin',
        element: <Navigate to="/admin/" replace />,
      },
      {
        path: '*',
        element: <Navigate to="/admin/" replace />,
      },
    ],
  },
], {
  basename: '/',
});

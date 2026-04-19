import { lazy, Suspense } from 'react';
import { createBrowserRouter, Navigate } from 'react-router-dom';
import { ProtectedRoute, GuestRoute, RouterAuthProvider } from '@/contexts/AuthContext';
import Login from '@/pages/Login';
import Layout from '@/components/Layout';

const Dashboard = lazy(() => import('@/pages/Dashboard'));
const ContentList = lazy(() => import('@/pages/ContentList'));
const ContentEdit = lazy(() => import('@/pages/ContentEdit'));
const ContentTypeList = lazy(() => import('@/pages/ContentTypeList'));
const ContentTypeBuilder = lazy(() => import('@/pages/ContentTypeBuilder'));
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
          { path: 'content/:contentType', element: <LazyPage Component={ContentList} /> },
          { path: 'content/:contentType/new', element: <LazyPage Component={ContentEdit} /> },
          { path: 'content/:contentType/:id', element: <LazyPage Component={ContentEdit} /> },
          { path: 'content-types', element: <LazyPage Component={ContentTypeList} /> },
          { path: 'content-types/new', element: <LazyPage Component={ContentTypeBuilder} /> },
          { path: 'content-types/:name', element: <LazyPage Component={ContentTypeBuilder} /> },
          { path: 'media', element: <LazyPage Component={MediaLibrary} /> },
          { path: 'users', element: <LazyPage Component={UserManagement} /> },
          { path: 'roles', element: <LazyPage Component={RoleManagement} /> },
          { path: 'plugins', element: <LazyPage Component={PluginManagement} /> },
          { path: 'settings', element: <LazyPage Component={Settings} /> },
          { path: 'api-tokens', element: <LazyPage Component={ApiTokens} /> },
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

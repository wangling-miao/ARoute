import { useState, useEffect, useCallback, useMemo, Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useLocation, Outlet, Link } from 'react-router-dom';
import { Layout, Menu, Breadcrumb, Avatar, Dropdown, Switch } from '@arco-design/web-react';
import {
  LayoutDashboard,
  FileText,
  Layers,
  Image,
  Users,
  UserCog,
  Puzzle,
  Settings,
  Key,
  Sun,
  Moon,
  Globe,
  Menu as MenuIcon,
  X,
  LogOut,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react';
import { useAuth } from '@/contexts/AuthContext';
import { useTheme } from '@/contexts/ThemeContext';
import { usePermissions } from '@/hooks/usePermissions';
import { contentTypes as contentTypesApi } from '@/api/endpoints';
import type { ContentType } from '@/types';
import styles from './Layout.module.css';

const { Sider } = Layout;
const MenuItem = Menu.Item;
const SubMenu = Menu.SubMenu;

function getInitial(pathname: string): string {
  return pathname === '/admin/' ? 'D' : 'A';
}

export default function AdminLayout() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const { user, logout } = useAuth();
  const { canAny } = usePermissions();
  const { theme, toggleTheme } = useTheme();

  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [isMobile, setIsMobile] = useState(window.innerWidth < 768);
  const [ctList, setCtList] = useState<ContentType[]>([]);

  const contentNavItems = useMemo(() => {
    const order = ['page', 'post'];
    const hidden = new Set(['menu', 'category', 'tag']);
    return ctList
      .filter(ct => !hidden.has(ct.name))
      .sort((a, b) => {
        const ai = order.indexOf(a.name);
        const bi = order.indexOf(b.name);
        if (ai !== -1 && bi !== -1) return ai - bi;
        if (ai !== -1) return -1;
        if (bi !== -1) return 1;
        return a.display_name.localeCompare(b.display_name);
      });
  }, [ctList]);
  const [openKeys, setOpenKeys] = useState<string[]>([]);

  useEffect(() => {
    const onResize = () => {
      const mobile = window.innerWidth < 768;
      setIsMobile(mobile);
      if (mobile) {
        setCollapsed(false);
        setMobileOpen(false);
      } else {
        setMobileOpen(false);
      }
    };
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  useEffect(() => {
    contentTypesApi.list().then(setCtList).catch(() => {});
  }, []);

  useEffect(() => {
    if (location.pathname.startsWith('/admin/content/')) {
      setOpenKeys(prev => (prev.includes('content') ? prev : [...prev, 'content']));
    }
  }, [location.pathname]);

  const handleMenuClick = useCallback(
    (key: string) => {
      navigate(key);
      if (isMobile) setMobileOpen(false);
    },
    [navigate, isMobile],
  );

  const handleLogout = useCallback(() => {
    logout();
  }, [logout]);

  const handleLangChange = useCallback(() => {
    const next = i18n.language?.startsWith('zh') ? 'en' : 'zh';
    i18n.changeLanguage(next);
  }, [i18n]);

  const selectedKeys = useMemo(() => {
    const p = location.pathname;
    if (p === '/admin/' || p === '/admin') return ['/admin/'];
    if (p.startsWith('/admin/content-types')) return ['/admin/content-types'];
    if (p.startsWith('/admin/menus')) return ['/admin/menus'];
    if (p.startsWith('/admin/content/')) {
      const seg = p.split('/');
      if (seg[3]) return [`/admin/content/${seg[3]}`];
      return ['/admin/content'];
    }
    if (p.startsWith('/admin/media')) return ['/admin/media'];
    if (p.startsWith('/admin/users')) return ['/admin/users'];
    if (p.startsWith('/admin/roles')) return ['/admin/roles'];
    if (p.startsWith('/admin/plugins')) return ['/admin/plugins'];
    if (p.startsWith('/admin/settings')) return ['/admin/settings'];
    if (p.startsWith('/admin/api-tokens')) return ['/admin/api-tokens'];
    return ['/admin/'];
  }, [location.pathname]);

  const breadcrumbs = useMemo(() => {
    const p = location.pathname;
    const items: Array<{ label: string; path?: string }> = [];

    if (p === '/admin/' || p === '/admin') {
      return [{ label: t('nav.dashboard') }];
    }

    items.push({ label: t('nav.dashboard'), path: '/admin/' });

    if (p.startsWith('/admin/content/')) {
      items.push({ label: t('nav.content') });
      const seg = p.replace('/admin/content/', '').split('/');
      if (seg[0]) {
        const ct = ctList.find(c => c.name === seg[0]);
        items.push({
          label: ct ? t(`content_type_names.${ct.name}`, ct.display_name) : seg[0],
          path: `/admin/content/${seg[0]}`,
        });
      }
      if (seg[1] === 'new') {
        items.push({ label: t('common.create') });
      } else if (seg[1]) {
        items.push({ label: t('common.edit') });
      }
      return items;
    }

    const map: Record<string, string> = {
      '/admin/content-types': t('nav.content_types'),
      '/admin/menus': t('nav.menus'),
      '/admin/media': t('nav.media'),
      '/admin/users': t('nav.users'),
      '/admin/roles': t('nav.roles'),
      '/admin/plugins': t('nav.plugins'),
      '/admin/settings': t('nav.settings'),
      '/admin/api-tokens': t('nav.api_tokens'),
    };

    for (const prefix of Object.keys(map)) {
      if (p.startsWith(prefix)) {
        items.push({ label: map[prefix], path: prefix });
        if (p !== prefix) {
          const rest = p.replace(prefix + '/', '');
          if (rest === 'new') {
            items.push({ label: t('common.create') });
          } else if (rest) {
            items.push({ label: t('common.edit') });
          }
        }
        break;
      }
    }

    return items;
  }, [location.pathname, t, ctList]);

  const userDropdown = (
    <Menu>
      <MenuItem key="profile" onClick={() => navigate('/admin/settings')}>
        {t('layout.profile')}
      </MenuItem>
      <MenuItem key="logout" onClick={handleLogout}>
        <span style={{ color: 'var(--color-danger)' }}>{t('nav.logout')}</span>
      </MenuItem>
    </Menu>
  );

  const sidebarContent = (
    <>
      <Link to="/admin/" className={styles.sidebarBrand}>
        <div className={styles.sidebarBrandIcon}>A</div>
        {!collapsed && <span className={styles.sidebarBrandText}>ARoute</span>}
      </Link>

      <div className={styles.sidebarMenuWrap}>
        <Menu
          theme={theme === 'dark' ? 'dark' : 'light'}
          mode="vertical"
          collapse={collapsed}
          selectedKeys={selectedKeys}
          openKeys={openKeys}
          onClickMenuItem={handleMenuClick}
          onClickSubMenu={(_key: string, newOpenKeys: string[]) => setOpenKeys(newOpenKeys)}
          style={{ background: 'transparent' }}
        >
          <MenuItem key="/admin/">
            <span className={styles.menuIcon}><LayoutDashboard size={18} /></span>
            {!collapsed && t('nav.dashboard')}
          </MenuItem>

          {canAny([{ resource: 'content', action: 'read' }]) && (
          <SubMenu
            key="content"
            title={
              <span>
                <span className={styles.menuIcon}><FileText size={18} /></span>
                {!collapsed && t('nav.content')}
              </span>
            }
          >
            {contentNavItems.map(ct => (
              <MenuItem key={`/admin/content/${ct.name}`}>
                {t(`content_type_names.${ct.name}`, ct.display_name)}
              </MenuItem>
            ))}
          </SubMenu>
          )}

          {canAny([{ resource: 'content_types', action: 'read' }]) && (
          <MenuItem key="/admin/content-types">
            <span className={styles.menuIcon}><Layers size={18} /></span>
            {!collapsed && t('nav.content_types')}
          </MenuItem>
          )}

          {canAny([{ resource: 'menus', action: 'read' }]) && (
          <MenuItem key="/admin/menus">
            <span className={styles.menuIcon}><MenuIcon size={18} /></span>
            {!collapsed && t('nav.menus')}
          </MenuItem>
          )}

          {canAny([{ resource: 'media', action: 'read' }]) && (
          <MenuItem key="/admin/media">
            <span className={styles.menuIcon}><Image size={18} /></span>
            {!collapsed && t('nav.media')}
          </MenuItem>
          )}

          {canAny([{ resource: 'users', action: 'read' }]) && (
          <MenuItem key="/admin/users">
            <span className={styles.menuIcon}><Users size={18} /></span>
            {!collapsed && t('nav.users')}
          </MenuItem>
          )}

          {canAny([{ resource: 'roles', action: 'read' }]) && (
          <MenuItem key="/admin/roles">
            <span className={styles.menuIcon}><UserCog size={18} /></span>
            {!collapsed && t('nav.roles')}
          </MenuItem>
          )}

          {canAny([{ resource: 'plugins', action: 'read' }]) && (
          <MenuItem key="/admin/plugins">
            <span className={styles.menuIcon}><Puzzle size={18} /></span>
            {!collapsed && t('nav.plugins')}
          </MenuItem>
          )}

          {canAny([{ resource: 'settings', action: 'read' }]) && (
          <MenuItem key="/admin/settings">
            <span className={styles.menuIcon}><Settings size={18} /></span>
            {!collapsed && t('nav.settings')}
          </MenuItem>
          )}

          {/* API Tokens — temporarily hidden */}
          {/* canAny([{ resource: 'api_tokens', action: 'read' }]) && (
          <MenuItem key="/admin/api-tokens">
            <span className={styles.menuIcon}><Key size={18} /></span>
            {!collapsed && t('nav.api_tokens')}
          </MenuItem>
          ) */}
        </Menu>
      </div>

      {!isMobile && (
        <div className={styles.sidebarFooter}>
          <button
            type="button"
            className={styles.collapseBtn}
            onClick={() => setCollapsed(c => !c)}
            aria-label={collapsed ? t('nav.expand') : t('nav.collapse')}
          >
            {collapsed ? <ChevronRight size={18} /> : <ChevronLeft size={18} />}
          </button>
        </div>
      )}
    </>
  );

  return (
    <Layout className={styles.layoutShell}>
      <div className={styles.meshBg} aria-hidden="true">
        <div className={`${styles.meshOrb} ${styles.meshOrb1}`} />
        <div className={`${styles.meshOrb} ${styles.meshOrb2}`} />
        <div className={`${styles.meshOrb} ${styles.meshOrb3}`} />
      </div>

      {isMobile && mobileOpen && (
        <button
          type="button"
          className={styles.mobileOverlay}
          onClick={() => setMobileOpen(false)}
          aria-label={t('common.close')}
        />
      )}

      {isMobile ? (
        mobileOpen && (
          <Sider
            width={240}
            collapsed={false}
            className={styles.sidebar}
            trigger={null}
            style={{
              position: 'fixed',
              top: 0,
              left: 0,
              bottom: 0,
              zIndex: 100,
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'flex-end', padding: '12px 12px 0' }}>
              <button
                type="button"
                onClick={() => setMobileOpen(false)}
                style={{
                  background: 'var(--glass-bg)',
                  border: 'none',
                  color: 'var(--color-sidebar-text)',
                  borderRadius: 'var(--radius-md)',
                  width: 32,
                  height: 32,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  cursor: 'pointer',
                }}
              >
                <X size={18} />
              </button>
            </div>
            {sidebarContent}
          </Sider>
        )
      ) : (
        <Sider
          collapsed={collapsed}
          collapsible
          width={240}
          collapsedWidth={60}
          className={styles.sidebar}
          trigger={null}
        >
          {sidebarContent}
        </Sider>
      )}

      <Layout className={styles.mainArea}>
        <header className={styles.topBar}>
          <div className={styles.topBarLeft}>
            {isMobile && (
              <button
                type="button"
                className={styles.mobileMenuBtn}
                onClick={() => setMobileOpen(true)}
                aria-label={t('layout.menu_toggle')}
              >
                <MenuIcon size={20} />
              </button>
            )}
            <Breadcrumb>
              {breadcrumbs.map((crumb, idx) => (
                <Breadcrumb.Item key={crumb.path || crumb.label}>
                  {crumb.path && idx < breadcrumbs.length - 1 ? (
                    <Link to={crumb.path}>{crumb.label}</Link>
                  ) : (
                    crumb.label
                  )}
                </Breadcrumb.Item>
              ))}
            </Breadcrumb>
          </div>

          <div className={styles.topBarRight}>
            <button
              type="button"
              className={styles.langBtn}
              onClick={handleLangChange}
              aria-label={`Switch language to ${i18n.language?.startsWith('zh') ? 'English' : '中文'}`}
            >
              <Globe size={15} />
              <span>{i18n.language?.startsWith('zh') ? t('layout.lang_zh') : t('layout.lang_en')}</span>
            </button>

            <Switch
              checked={theme === 'dark'}
              onChange={toggleTheme}
              checkedIcon={<Moon size={14} />}
              uncheckedIcon={<Sun size={14} />}
              style={{ margin: '0 4px' }}
              aria-label={t('layout.theme_toggle')}
            />

            <Dropdown droplist={userDropdown} trigger="click" position="br">
              <button type="button" className={styles.userBtn}>
                <Avatar size={30} style={{ backgroundColor: 'var(--color-primary)', fontSize: '0.8125rem' }}>
                  {user?.username?.[0]?.toUpperCase() || getInitial(location.pathname)}
                </Avatar>
                {user?.username && <span className={styles.userBtnName}>{user.username}</span>}
                <LogOut size={14} className={styles.dropdownIcon} />
              </button>
            </Dropdown>
          </div>
        </header>

        <Layout.Content className={styles.contentArea}>
          <div className={styles.contentInner}>
            <Suspense
              fallback={
                <div className="page-loading">
                  <div className="loading-spinner" />
                </div>
              }
            >
              <Outlet />
            </Suspense>
          </div>
        </Layout.Content>
      </Layout>
    </Layout>
  );
}

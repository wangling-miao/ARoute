import { useEffect, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Card, Tag } from '@arco-design/web-react';
import {
  FileText,
  Activity,
  Clock,
  Database,
  Plug,
  Gauge,
  Plus,
  UserPlus,
  ExternalLink,
  Inbox,
} from 'lucide-react';
import { dashboard, settings } from '@/api/endpoints';
import type { DashboardStats, ActivityItem } from '@/types';
import styles from './Dashboard.module.css';

const ICON_COLORS = ['statIconBlue', 'statIconGreen', 'statIconOrange', 'statIconPurple', 'statIconRed', 'statIconCyan'] as const;
const ICONS = [FileText, Activity, Database, Plug, Gauge, FileText] as const;

function formatRelativeTime(dateStr: string, t: (key: string) => string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const diffMs = now - then;
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHour / 24);

  if (diffSec < 60) return t('dashboard.time_just_now');
  if (diffMin < 60) return (t as any)('dashboard.time_minutes_ago', { count: diffMin });
  if (diffHour < 24) return (t as any)('dashboard.time_hours_ago', { count: diffHour });
  if (diffDay < 7) return (t as any)('dashboard.time_days_ago', { count: diffDay });
  return new Date(dateStr).toLocaleDateString();
}

function getActionIcon(action: string) {
  switch (action) {
    case 'create': return <Plus size={15} />;
    case 'update': return <Activity size={15} />;
    case 'delete': return <FileText size={15} />;
    default: return <Activity size={15} />;
  }
}

function LoadingSkeleton() {
  return (
    <div>
      <div className={styles.statsGrid}>
        {['a', 'b', 'c', 'd'].map((key) => (
          <div key={key} className={`skeleton ${styles.skeletonStatCard}`} />
        ))}
      </div>
      <div className={styles.section}>
        <div className={`skeleton skeleton-title`} style={{ marginBottom: 16 }} />
        <div className={`skeleton ${styles.skeletonActivityRow}`} style={{ height: 200, borderRadius: 12 }} />
      </div>
    </div>
  );
}

function SystemStatusSection({ stats, onNewContent, onNewUser, onViewSite }: {
  stats: DashboardStats;
  onNewContent: () => void;
  onNewUser: () => void;
  onViewSite: () => void;
}) {
  const { t } = useTranslation();

  const dbStatusClass = stats.system_status.database === 'healthy'
    ? styles.statusDotHealthy
    : stats.system_status.database === 'degraded'
      ? styles.statusDotDegraded
      : styles.statusDotDown;

  const dbStatusLabel = stats.system_status.database === 'healthy'
    ? t('dashboard.healthy')
    : stats.system_status.database === 'degraded'
      ? t('dashboard.degraded')
      : t('dashboard.down');

  const cachePercent = Math.round(stats.system_status.cache_hit_ratio * 100);

  return (
    <div className={styles.systemGrid}>
      <Card className={styles.systemCard} bordered={false}>
        <div className={styles.systemItem}>
          <Database size={18} style={{ color: 'var(--color-text-tertiary)' }} />
          <span className={styles.systemItemLabel}>{t('dashboard.database')}</span>
          <span className={`${styles.statusDot} ${dbStatusClass}`} />
          <span className={styles.statusText}>{dbStatusLabel}</span>
        </div>
        <div className={styles.systemItem}>
          <Plug size={18} style={{ color: 'var(--color-text-tertiary)' }} />
          <span className={styles.systemItemLabel}>{t('dashboard.plugin_count')}</span>
          <Tag color="blue" size="small">{stats.system_status.plugin_count}</Tag>
        </div>
      </Card>

      <Card className={styles.systemCard} bordered={false}>
        <div className={styles.systemItem}>
          <Gauge size={18} style={{ color: 'var(--color-text-tertiary)' }} />
          <span className={styles.systemItemLabel}>{t('dashboard.cache_hit_ratio')}</span>
          <span className={styles.cachePercent}>{cachePercent}%</span>
        </div>
        <div className={styles.cacheBarWrap}>
          <div className={styles.cacheBar}>
            <div className={styles.cacheBarFill} style={{ width: `${cachePercent}%` }} />
          </div>
        </div>
      </Card>

      <Card className={styles.systemCard} bordered={false}>
        <div className={styles.systemItem}>
          <FileText size={18} style={{ color: 'var(--color-text-tertiary)' }} />
          <span className={styles.systemItemLabel}>{t('dashboard.quick_actions')}</span>
        </div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          <Tag color="blue" size="small" style={{ cursor: 'pointer' }} onClick={onNewContent}>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
              <Plus size={12} /> {t('dashboard.new_content')}
            </span>
          </Tag>
          <Tag color="green" size="small" style={{ cursor: 'pointer' }} onClick={onNewUser}>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
              <UserPlus size={12} /> {t('dashboard.new_user')}
            </span>
          </Tag>
          <Tag color="purple" size="small" style={{ cursor: 'pointer' }} onClick={onViewSite}>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
              <ExternalLink size={12} /> {t('dashboard.view_site')}
            </span>
          </Tag>
        </div>
      </Card>
    </div>
  );
}

function ActivitySection({ items }: { items: ActivityItem[] }) {
  const { t } = useTranslation();

  if (items.length === 0) {
    return (
      <div className={styles.section}>
        <h3 className={styles.sectionTitle}>{t('dashboard.recent_activity')}</h3>
        <Card className={styles.activityCard} bordered={false}>
          <div className={styles.emptyActivity}>
            <Inbox size={40} className={styles.emptyActivityIcon} />
            <span className={styles.emptyActivityText}>{t('dashboard.no_activity')}</span>
          </div>
        </Card>
      </div>
    );
  }

  return (
    <div className={styles.section}>
      <h3 className={styles.sectionTitle}>{t('dashboard.recent_activity')}</h3>
      <Card className={styles.activityCard} bordered={false}>
        {items.map((item) => (
          <div key={item.id} className={styles.activityItem}>
            <div className={styles.activityIcon}>
              {getActionIcon(item.action)}
            </div>
            <div className={styles.activityInfo}>
              <div className={styles.activityAction}>
                {t(`dashboard.action_${item.action}`)} {item.resource_type}
              </div>
              <div className={styles.activityMeta}>
                ID: {item.resource_id.slice(0, 8)}…
              </div>
            </div>
            <span className={styles.activityTime}>
              <Clock size={12} style={{ marginRight: 4, verticalAlign: 'middle' }} />
              {formatRelativeTime(item.created_at, t)}
            </span>
          </div>
        ))}
      </Card>
    </div>
  );
}

export default function Dashboard() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [siteUrl, setSiteUrl] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchStats = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await dashboard.getStats();
      setStats(data);
    } catch {
      setError(t('common.error_occurred'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    settings.get().then((s) => setSiteUrl(s.site_url)).catch(() => {});
  }, []);

  useEffect(() => {
    fetchStats();
  }, [fetchStats]);

  if (loading) {
    return (
      <div className={styles.page}>
        <LoadingSkeleton />
      </div>
    );
  }

  if (error) {
    return (
      <div className={styles.page}>
        <div style={{ textAlign: 'center', padding: '48px 24px', color: 'var(--color-text-tertiary)' }}>
          <p>{error}</p>
          <button
            type="button"
            onClick={fetchStats}
            style={{ marginTop: 12, color: 'var(--color-primary)', background: 'none', border: 'none', cursor: 'pointer', fontWeight: 600 }}
          >
            {t('common.try_again')}
          </button>
        </div>
      </div>
    );
  }

  if (!stats) return null;

  const contentEntries = Object.entries(stats.content_counts);

  const handleNewContent = () => {
    const firstType = contentEntries.length > 0 ? contentEntries[0][0] : null;
    if (firstType) {
      navigate(`/admin/content/${firstType}/new`);
    } else {
      navigate('/admin/content-types/new');
    }
  };

  const handleNewUser = () => navigate('/admin/users');
  const handleViewSite = () => window.open(siteUrl || window.location.origin, '_blank');

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h2 className={styles.headerTitle}>{t('dashboard.title')}</h2>
      </div>

      {contentEntries.length > 0 && (
        <div className={styles.statsGrid}>
          {contentEntries.map(([type, count], idx) => {
            const IconComp = ICONS[idx % ICONS.length];
            const colorClass = ICON_COLORS[idx % ICON_COLORS.length];
            return (
              <Card key={type} className={styles.statCard} bordered={false}>
                <div className={`${styles.statIconWrap} ${styles[colorClass]}`}>
                  <IconComp size={22} />
                </div>
                <div className={styles.statCount}>{count}</div>
                <div className={styles.statLabel}>{type.replace(/_/g, ' ')}</div>
              </Card>
            );
          })}
        </div>
      )}

      <ActivitySection items={stats.recent_activity} />

      <div className={styles.section}>
        <h3 className={styles.sectionTitle}>{t('dashboard.system_status')}</h3>
        <SystemStatusSection stats={stats} onNewContent={handleNewContent} onNewUser={handleNewUser} onViewSite={handleViewSite} />
      </div>
    </div>
  );
}

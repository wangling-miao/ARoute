import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Spin, Typography, Switch } from '@arco-design/web-react';
import { Package, User } from 'lucide-react';
import { plugins } from '@/api/endpoints';
import { ApiError } from '@/api/client';
import { showError, showSuccess } from '@/components/Toast';
import { confirm } from '@/components/ConfirmDialog';
import { usePermissions } from '@/hooks/usePermissions';
import type { Plugin } from '@/types';
import styles from './PluginManagement.module.css';

const { Title } = Typography;

export default function PluginManagement() {
  const { t } = useTranslation();
  const { can } = usePermissions();
  const [pluginList, setPluginList] = useState<Plugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [expandedName, setExpandedName] = useState<string | null>(null);
  const [toggling, setToggling] = useState<string | null>(null);

  const fetchPlugins = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const data = await plugins.list();
      setPluginList(data);
    } catch (err) {
      setError(true);
      if (err instanceof ApiError) showError(err.message);
      else showError(t('common.error_occurred'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchPlugins();
  }, [fetchPlugins]);

  const handleToggle = (plugin: Plugin) => {
    if (plugin.enabled) {
      confirm({
        title: t('plugins.disable'),
        message: t('plugins.disable_confirm', { name: plugin.name }),
        danger: true,
        confirmText: t('plugins.disable'),
        cancelText: t('common.cancel'),
        onConfirm: async () => {
          setToggling(plugin.name);
          try {
            await plugins.disable(plugin.name);
            showSuccess(t('plugins.disabled_success', { name: plugin.name }));
            setPluginList((prev) =>
              prev.map((p) => (p.name === plugin.name ? { ...p, enabled: false } : p)),
            );
          } catch (err) {
            if (err instanceof ApiError) showError(err.message);
          } finally {
            setToggling(null);
          }
        },
      });
    } else {
      setToggling(plugin.name);
      plugins
        .enable(plugin.name)
        .then(() => {
          showSuccess(t('plugins.enabled_success', { name: plugin.name }));
          setPluginList((prev) =>
            prev.map((p) => (p.name === plugin.name ? { ...p, enabled: true } : p)),
          );
        })
        .catch((err) => {
          if (err instanceof ApiError) showError(err.message);
        })
        .finally(() => setToggling(null));
    }
  };

  if (loading) {
    return (
      <div className={styles.page}>
        <div className={styles.loadingWrap}><Spin size={40} /></div>
      </div>
    );
  }

  if (error) {
    return (
      <div className={styles.page}>
        <div className={styles.emptyState}>
          <p>{t('common.error_occurred')}</p>
          <Button onClick={fetchPlugins}>{t('common.try_again')}</Button>
        </div>
      </div>
    );
  }

  if (pluginList.length === 0) {
    return (
      <div className={styles.page}>
        <div className={styles.header}>
          <div className={styles.headerLeft}>
            <Title heading={5} style={{ margin: 0 }}>{t('plugins.title')}</Title>
          </div>
        </div>
        <div className={styles.emptyState}>
          <div className={styles.emptyIcon}><Package size={32} /></div>
          <div className={styles.emptyTitle}>{t('plugins.no_plugins')}</div>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <Title heading={5} style={{ margin: 0 }}>{t('plugins.title')}</Title>
        </div>
      </div>

      <div className={styles.pluginGrid}>
        {pluginList.map((plugin) => {
          const isExpanded = expandedName === plugin.name;
          return (
            <div
              key={plugin.name}
              className={`${styles.pluginCard} ${isExpanded ? styles.pluginCardExpanded : ''}`}
            >
              <div className={styles.pluginHeader}>
                <div className={styles.pluginInfo}>
                  <button
                    type="button"
                    className={styles.pluginName}
                    onClick={() => setExpandedName(isExpanded ? null : plugin.name)}
                  >
                    {plugin.name}
                  </button>
                  <div className={styles.pluginMeta}>
                    <span className={styles.pluginVersion}>v{plugin.version}</span>
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                      <User size={12} /> {plugin.author}
                    </span>
                  </div>
                </div>
                <Switch
                  checked={plugin.enabled}
                  onChange={() => handleToggle(plugin)}
                  loading={toggling === plugin.name}
                  disabled={!can('plugins', plugin.enabled ? 'disable' : 'enable')}
                />
              </div>

              <div className={styles.pluginDesc}>{plugin.description}</div>

              {isExpanded && (
                <div className={styles.pluginDetails}>
                  <div className={styles.detailRow}>
                    <span className={styles.detailLabel}>{t('common.name')}</span>
                    <span className={styles.detailValue}>{plugin.name}</span>
                  </div>
                  <div className={styles.detailRow}>
                    <span className={styles.detailLabel}>{t('plugins.version')}</span>
                    <span className={styles.detailValue}>{plugin.version}</span>
                  </div>
                  <div className={styles.detailRow}>
                    <span className={styles.detailLabel}>{t('plugins.author')}</span>
                    <span className={styles.detailValue}>{plugin.author}</span>
                  </div>
                  <div className={styles.detailRow}>
                    <span className={styles.detailLabel}>{t('common.status')}</span>
                    <span className={styles.detailValue}>
                      {plugin.enabled ? t('common.enabled') : t('common.disabled')}
                    </span>
                  </div>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

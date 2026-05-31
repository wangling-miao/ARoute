import { useState, useEffect, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Spin, Typography, Switch } from '@arco-design/web-react';
import { Package, User, Upload, Eye, EyeOff, Zap, Shield, Gauge } from 'lucide-react';
import { plugins as pluginsApi } from '@/api/endpoints';
import { ApiError } from '@/api/client';
import { showError, showSuccess } from '@/components/Toast';
import { confirm } from '@/components/ConfirmDialog';
import { usePermissions } from '@/hooks/usePermissions';
import type { Plugin } from '@/types';
import styles from './PluginManagement.module.css';

const { Title } = Typography;

function stateLabel(t: (k: string) => string, state: string) {
  switch (state) {
    case 'active': return t('plugins.state_active');
    case 'stopped': return t('plugins.state_stopped');
    case 'failed': return t('plugins.state_failed');
    default: return t('plugins.state_not_loaded');
  }
}

function stateColor(state: string) {
  switch (state) {
    case 'active': return 'var(--color-success, #00b42a)';
    case 'stopped': return 'var(--color-text-tertiary)';
    case 'failed': return 'var(--color-danger, #f53f3f)';
    default: return 'var(--color-warning, #ff7d00)';
  }
}

function trustColor(state?: string, score?: number) {
  if (state === 'disabled' || (score ?? 0) >= 80) return 'var(--color-danger, #f53f3f)';
  if (state === 'quarantined' || (score ?? 0) >= 60) return 'var(--color-danger, #f53f3f)';
  if (state === 'guarded' || state === 'pending_review' || (score ?? 0) >= 30) return 'var(--color-warning, #ff7d00)';
  return 'var(--color-success, #00b42a)';
}

function engineLabel(plugin: Plugin) {
  const engine = plugin.engine || 'native';
  if (engine === 'grpc' || engine === 'l2') return 'L2 gRPC';
  if (engine === 'wasm' || engine === 'l3') return 'L3 Wasm';
  return 'L1 Native';
}

export default function PluginManagement() {
  const { t } = useTranslation();
  const { can } = usePermissions();
  const [pluginList, setPluginList] = useState<Plugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [expandedName, setExpandedName] = useState<string | null>(null);
  const [toggling, setToggling] = useState<string | null>(null);
  const [showSystem, setShowSystem] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [dragActive, setDragActive] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const fetchPlugins = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const data = await pluginsApi.list();
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
    if (plugin.is_system) return;

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
            await pluginsApi.disable(plugin.name);
            showSuccess(t('plugins.disabled_success', { name: plugin.name }));
            setPluginList((prev) =>
              prev.map((p) => (p.name === plugin.name ? { ...p, enabled: false, state: 'stopped' } : p)),
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
      pluginsApi
        .enable(plugin.name)
        .then(() => {
          showSuccess(t('plugins.enabled_success', { name: plugin.name }));
          setPluginList((prev) =>
            prev.map((p) => (p.name === plugin.name ? { ...p, enabled: true, state: 'active' } : p)),
          );
        })
        .catch((err) => {
          if (err instanceof ApiError) showError(err.message);
        })
        .finally(() => setToggling(null));
    }
  };

  const handleUpload = async (file: File) => {
    setUploading(true);
    setUploadProgress(0);
    try {
      const result = await pluginsApi.upload(file, (pct) => setUploadProgress(pct));
      showSuccess(t('plugins.install_success', { name: result.name }));
      fetchPlugins();
    } catch (err) {
      if (err instanceof ApiError) showError(err.message);
      else showError(t('plugins.upload_invalid'));
    } finally {
      setUploading(false);
      setUploadProgress(0);
    }
  };

  const onFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) handleUpload(file);
    e.target.value = '';
  };

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragActive(false);
    const file = e.dataTransfer.files[0];
    if (file) handleUpload(file);
  };

  const onDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setDragActive(true);
  };

  const onDragLeave = () => setDragActive(false);

  const canUpload = can('plugins', 'enable');
  const filteredList = pluginList.filter((p) => showSystem || !p.is_system);

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

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <Title heading={5} style={{ margin: 0 }}>{t('plugins.title')}</Title>
          <span className={styles.countBadge}>{pluginList.length}</span>
        </div>
        <div className={styles.headerRight}>
          <Button
            size="small"
            icon={showSystem ? <EyeOff size={14} /> : <Eye size={14} />}
            onClick={() => setShowSystem((v) => !v)}
          >
            {showSystem ? t('plugins.hide_system') : t('plugins.show_system')}
          </Button>
          {canUpload && (
            <>
              <input
                ref={fileInputRef}
                type="file"
                accept=".zip,.wasm,.tar.gz"
                style={{ display: 'none' }}
                onChange={onFileChange}
              />
              <Button
                type="primary"
                size="small"
                icon={<Upload size={14} />}
                loading={uploading}
                onClick={() => fileInputRef.current?.click()}
              >
                {t('plugins.upload')}
              </Button>
            </>
          )}
        </div>
      </div>

      {uploading && (
        <div className={styles.progressBar}>
          <div className={styles.progressFill} style={{ width: `${uploadProgress}%` }} />
        </div>
      )}

      <div
        className={`${styles.dropzone} ${dragActive ? styles.dropzoneActive : ''}`}
        onDrop={onDrop}
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
      >
        {pluginList.length === 0 ? (
          <div className={styles.emptyState}>
            <div className={styles.emptyIcon}><Package size={32} /></div>
            <div className={styles.emptyTitle}>{t('plugins.no_plugins')}</div>
          </div>
        ) : filteredList.length === 0 ? (
          <div className={styles.emptyState}>
            <div className={styles.emptyIcon}><Eye size={32} /></div>
            <div className={styles.emptyTitle}>
              {showSystem ? t('plugins.no_plugins') : t('plugins.show_system')}
            </div>
          </div>
        ) : (
          <div className={styles.pluginGrid}>
            {filteredList.map((plugin) => {
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
                        {plugin.is_system && (
                          <span className={styles.systemBadge}>
                            <Zap size={10} /> {t('plugins.system')}
                          </span>
                        )}
                        <span className={styles.systemBadge}>
                          <Shield size={10} /> {engineLabel(plugin)}
                        </span>
                      </div>
                    </div>
                    <Switch
                      checked={plugin.enabled}
                      onChange={() => handleToggle(plugin)}
                      loading={toggling === plugin.name}
                      disabled={plugin.is_system || !can('plugins', plugin.enabled ? 'disable' : 'enable')}
                    />
                  </div>

                  <div className={styles.pluginDesc}>{plugin.description}</div>

                  <div className={styles.stateRow}>
                    <span
                      className={styles.stateIndicator}
                      style={{ background: stateColor(plugin.state) }}
                    />
                    <span className={styles.stateText}>{stateLabel(t, plugin.state)}</span>
                    <span
                      className={styles.stateText}
                      style={{ marginLeft: 12, color: trustColor(plugin.trust_state, plugin.risk_score) }}
                    >
                      <Gauge size={12} /> {plugin.trust_state || 'allow'} · {plugin.risk_score ?? 0}
                    </span>
                  </div>

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
                      <div className={styles.detailRow}>
                        <span className={styles.detailLabel}>Trust</span>
                        <span className={styles.detailValue}>
                          {plugin.effective_trust || plugin.trust_level || 'L1'} / {plugin.trust_state || 'allow'}
                        </span>
                      </div>
                      <div className={styles.detailRow}>
                        <span className={styles.detailLabel}>Risk</span>
                        <span className={styles.detailValue} style={{ color: trustColor(plugin.trust_state, plugin.risk_score) }}>
                          {plugin.risk_score ?? 0}
                        </span>
                      </div>
                      <div className={styles.detailRow}>
                        <span className={styles.detailLabel}>Capabilities</span>
                        <span className={styles.detailValue}>
                          {(plugin.capability_grants || plugin.capabilities || []).slice(0, 4).join(', ') || '-'}
                        </span>
                      </div>
                      {plugin.last_decision && (
                        <div className={styles.detailRow}>
                          <span className={styles.detailLabel}>Decision</span>
                          <span className={styles.detailValue}>{plugin.last_decision.reason}</span>
                        </div>
                      )}
                      <div className={styles.detailRow}>
                        <span className={styles.detailLabel}>{t('plugins.state_active')}</span>
                        <span className={styles.detailValue} style={{ color: stateColor(plugin.state) }}>
                          {stateLabel(t, plugin.state)}
                        </span>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

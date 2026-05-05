import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Spin, Typography, Tag } from '@arco-design/web-react';
import { Paintbrush, Monitor, Cpu, User, Check } from 'lucide-react';
import { themes as themesApi, adminVariants as variantsApi } from '@/api/endpoints';
import { ApiError } from '@/api/client';
import { showError, showSuccess } from '@/components/Toast';
import { confirm } from '@/components/ConfirmDialog';
import type { ThemeInfo, AdminVariant } from '@/types';
import styles from './Appearance.module.css';

const { Title } = Typography;

export default function Appearance() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [themeList, setThemeList] = useState<ThemeInfo[]>([]);
  const [variantList, setVariantList] = useState<AdminVariant[]>([]);
  const [activatingTheme, setActivatingTheme] = useState<string | null>(null);
  const [switchingVariant, setSwitchingVariant] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const [ts, vs] = await Promise.all([
        themesApi.list(),
        variantsApi.list(),
      ]);
      setThemeList(ts || []);
      setVariantList(vs || []);
    } catch (err) {
      if (err instanceof ApiError) showError(err.message);
      else showError(t('common.error_occurred'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const handleActivateTheme = async (slug: string) => {
    setActivatingTheme(slug);
    try {
      await themesApi.setActive(slug);
      showSuccess(t('appearance.theme_switched'));
      setThemeList(prev => prev.map(th => ({ ...th, active: th.slug === slug })));
    } catch (err) {
      if (err instanceof ApiError) showError(err.message);
      else showError(t('common.error_occurred'));
    } finally {
      setActivatingTheme(null);
    }
  };

  const handleSwitchVariant = (variant: string) => {
    confirm({
      title: t('appearance.admin_switch_confirm_title'),
      message: t('appearance.admin_switch_confirm_message'),
      confirmText: t('common.confirm'),
      onConfirm: async () => {
        setSwitchingVariant(variant);
        try {
          await variantsApi.set(variant);
          showSuccess(t('appearance.admin_switched'));
          setTimeout(() => window.location.reload(), 500);
        } catch (err) {
          if (err instanceof ApiError) showError(err.message);
          else showError(t('common.error_occurred'));
          setSwitchingVariant(null);
        }
      },
    });
  };

  if (loading) {
    return (
      <div className={styles.page}>
        <div className={styles.loadingWrap}><Spin size={40} /></div>
      </div>
    );
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <Title heading={5} style={{ margin: 0, display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            <Paintbrush size={20} /> {t('appearance.title')}
          </Title>
        </div>
      </div>

      {/* Frontend Themes */}
      <div className={styles.section}>
        <div className={styles.sectionTitle}>
          <Monitor size={18} /> {t('appearance.frontend_themes')}
        </div>
        <p className={styles.sectionDesc}>{t('appearance.frontend_themes_desc')}</p>

        {themeList.length === 0 ? (
          <div className={styles.empty}>{t('appearance.no_themes')}</div>
        ) : (
          <div className={styles.grid}>
            {themeList.map(theme => (
              <div
                key={theme.slug}
                className={`${styles.card} ${theme.active ? styles.cardActive : ''}`}
              >
                <div className={styles.cardHeader}>
                  <span className={styles.cardName}>{theme.name || theme.slug}</span>
                  {theme.active && (
                    <Tag className={styles.cardBadge} color="arcoblue">
                      <Check size={12} /> {t('appearance.active_badge')}
                    </Tag>
                  )}
                </div>
                {theme.description && (
                  <p className={styles.cardDesc}>{theme.description}</p>
                )}
                <div className={styles.cardMeta}>
                  {theme.engine && (
                    <span><Cpu size={12} /> {t('appearance.engine_type')}: {theme.engine}</span>
                  )}
                  {theme.version && (
                    <span>v{theme.version}</span>
                  )}
                  {theme.author && (
                    <span><User size={12} /> {theme.author}</span>
                  )}
                </div>
                {!theme.active && (
                  <div className={styles.cardFooter}>
                    <Button
                      type="primary"
                      size="small"
                      loading={activatingTheme === theme.slug}
                      onClick={() => handleActivateTheme(theme.slug)}
                    >
                      {t('appearance.activate')}
                    </Button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Admin Variants */}
      <div className={styles.section}>
        <div className={styles.sectionTitle}>
          <Paintbrush size={18} /> {t('appearance.admin_variants')}
        </div>
        <p className={styles.sectionDesc}>{t('appearance.admin_variants_desc')}</p>

        {variantList.length === 0 ? (
          <div className={styles.empty}>{t('appearance.no_variants')}</div>
        ) : (
          <div className={styles.grid}>
            {variantList.map(v => (
              <div
                key={v.variant}
                className={`${styles.card} ${v.active ? styles.cardActive : ''}`}
              >
                <div className={styles.cardHeader}>
                  <span className={styles.cardName}>{v.name || v.variant}</span>
                  {v.active && (
                    <Tag className={styles.cardBadge} color="arcoblue">
                      <Check size={12} /> {t('appearance.active_badge')}
                    </Tag>
                  )}
                </div>
                {v.description && (
                  <p className={styles.cardDesc}>{v.description}</p>
                )}
                <div className={styles.cardMeta}>
                  {v.version && <span>v{v.version}</span>}
                </div>
                {!v.active && (
                  <div className={styles.cardFooter}>
                    <Button
                      type="primary"
                      size="small"
                      loading={switchingVariant === v.variant}
                      onClick={() => handleSwitchVariant(v.variant)}
                    >
                      {t('appearance.switch_to')}
                    </Button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

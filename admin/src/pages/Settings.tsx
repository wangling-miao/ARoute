import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button, Input, InputNumber, Spin, Typography, Form, Select,
} from '@arco-design/web-react';
import { Settings as SettingsIcon, Mail, Save, Globe, Clock } from 'lucide-react';
import { settings as settingsApi } from '@/api/endpoints';
import { ApiError } from '@/api/client';
import { showError, showSuccess } from '@/components/Toast';
import type { Settings } from '@/types';
import styles from './Settings.module.css';

const { Title } = Typography;

const TIMEZONES = [
  'UTC', 'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
  'Europe/London', 'Europe/Paris', 'Europe/Berlin', 'Asia/Shanghai', 'Asia/Tokyo',
  'Asia/Kolkata', 'Australia/Sydney',
];

const LANGUAGES = [
  { label: 'English', value: 'en' },
  { label: '中文', value: 'zh' },
];

type Section = 'general' | 'email';

export default function SettingsPage() {
  const { t } = useTranslation();
  const [activeSection, setActiveSection] = useState<Section>('general');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [formData, setFormData] = useState<Settings>({
    site_name: '',
    site_url: '',
    language: 'en',
    timezone: 'UTC',
    smtp_host: '',
    smtp_port: 587,
    smtp_username: '',
    sender_email: '',
  });

  const [form] = Form.useForm();

  const fetchSettings = useCallback(async () => {
    setLoading(true);
    try {
      const data = await settingsApi.get();
      setFormData(data);
      form.setFieldsValue(data);
    } catch (err) {
      if (err instanceof ApiError) showError(err.message);
      else showError(t('common.error_occurred'));
    } finally {
      setLoading(false);
    }
  }, [form, t]);

  useEffect(() => {
    fetchSettings();
  }, [fetchSettings]);

  const handleSave = async () => {
    try {
      const values = await form.validate();
      setSaving(true);
      const updated = await settingsApi.update(values);
      setFormData(updated);
      showSuccess(t('settings.save_success'));
    } catch (err) {
      if (err instanceof ApiError) showError(err.message);
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className={styles.page}>
        <div className={styles.loadingWrap}><Spin size={40} /></div>
      </div>
    );
  }

  const sections: { key: Section; icon: React.ReactNode; label: string }[] = [
    { key: 'general', icon: <Globe size={16} />, label: t('settings.general') },
    { key: 'email', icon: <Mail size={16} />, label: t('settings.email') },
  ];

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <Title heading={5} style={{ margin: 0, display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            <SettingsIcon size={20} /> {t('settings.title')}
          </Title>
        </div>
      </div>

      <div className={styles.body}>
        <div className={styles.navCard}>
          {sections.map((s) => (
            <button
              key={s.key}
              type="button"
              className={`${styles.navItem} ${activeSection === s.key ? styles.navItemActive : ''}`}
              onClick={() => setActiveSection(s.key)}
            >
              {s.icon} {s.label}
            </button>
          ))}
        </div>

        <Form form={form} layout="vertical" initialValues={formData}>
          <div className={styles.sections}>
            {activeSection === 'general' && (
              <div className={styles.section}>
                <div className={styles.sectionTitle}>
                  <Globe size={18} /> {t('settings.general')}
                </div>
                <div className={styles.formRow}>
                  <Form.Item label={t('settings.site_name')} field="site_name" rules={[{ required: true }]}>
                    <Input />
                  </Form.Item>
                  <Form.Item
                    label={t('settings.site_url')}
                    field="site_url"
                    rules={[
                      { required: true },
                      {
                        validator: (val, cb) => {
                          if (val && !/^https?:\/\/.+/.test(val)) {
                            cb('Must be a valid URL starting with http:// or https://');
                          }
                          cb();
                        },
                      },
                    ]}
                  >
                    <Input placeholder="https://example.com" />
                  </Form.Item>
                </div>
                <div className={styles.formRow}>
                  <Form.Item label={t('settings.language')} field="language">
                    <Select>
                      {LANGUAGES.map((l) => (
                        <Select.Option key={l.value} value={l.value}>{l.label}</Select.Option>
                      ))}
                    </Select>
                  </Form.Item>
                  <Form.Item label={t('settings.timezone')} field="timezone">
                    <Select showSearch>
                      {TIMEZONES.map((tz) => (
                        <Select.Option key={tz} value={tz}>
                          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                            <Clock size={12} /> {tz}
                          </span>
                        </Select.Option>
                      ))}
                    </Select>
                  </Form.Item>
                </div>
              </div>
            )}

            {activeSection === 'email' && (
              <div className={styles.section}>
                <div className={styles.sectionTitle}>
                  <Mail size={18} /> {t('settings.email')}
                </div>
                <div className={styles.formRow}>
                  <Form.Item label={t('settings.smtp_host')} field="smtp_host">
                    <Input placeholder="smtp.example.com" />
                  </Form.Item>
                  <Form.Item
                    label={t('settings.smtp_port')}
                    field="smtp_port"
                    rules={[
                      { type: 'number', min: 1, max: 65535 },
                    ]}
                  >
                    <InputNumber min={1} max={65535} placeholder="587" style={{ width: '100%' }} />
                  </Form.Item>
                </div>
                <div className={styles.formRow}>
                  <Form.Item label={t('settings.smtp_username')} field="smtp_username">
                    <Input />
                  </Form.Item>
                  <Form.Item
                    label={t('settings.sender_email')}
                    field="sender_email"
                    rules={[{ type: 'email' }]}
                  >
                    <Input placeholder="noreply@example.com" />
                  </Form.Item>
                </div>
              </div>
            )}
          </div>

          <div className={styles.footer}>
            <Button
              type="primary"
              icon={<Save size={16} />}
              loading={saving}
              onClick={handleSave}
            >
              {t('common.save')}
            </Button>
          </div>
        </Form>
      </div>
    </div>
  );
}

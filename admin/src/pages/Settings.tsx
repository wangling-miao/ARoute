import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button, Input, Spin, Typography, Form, Select,
} from '@arco-design/web-react';
import { Settings as SettingsIcon, Save, Globe, Clock, Mail } from 'lucide-react';
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

export default function SettingsPage() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [activeSection, setActiveSection] = useState<'general' | 'email'>('general');
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
        <Form form={form} layout="vertical" initialValues={formData}>
          <div className={styles.sectionNav}>
            <Button
              type={activeSection === 'general' ? 'primary' : 'default'}
              icon={<Globe size={16} />}
              onClick={() => setActiveSection('general')}
            >
              {t('settings.general')}
            </Button>
            <Button
              type={activeSection === 'email' ? 'primary' : 'default'}
              icon={<Mail size={16} />}
              onClick={() => setActiveSection('email')}
            >
              {t('settings.email')}
            </Button>
          </div>

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
                    <Input />
                  </Form.Item>
                  <Form.Item label={t('settings.smtp_port')} field="smtp_port">
                    <Input />
                  </Form.Item>
                </div>
                <div className={styles.formRow}>
                  <Form.Item label={t('settings.smtp_username')} field="smtp_username">
                    <Input />
                  </Form.Item>
                  <Form.Item label={t('settings.sender_email')} field="sender_email">
                    <Input />
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

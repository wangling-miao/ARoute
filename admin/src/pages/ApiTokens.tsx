import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button, Table, Spin, Typography, Modal, Input, Form, Tag,
  DatePicker,
} from '@arco-design/web-react';
import { Plus, Key, Trash2, Copy, AlertTriangle } from 'lucide-react';
import { apiTokens } from '@/api/endpoints';
import { ApiError } from '@/api/client';
import { showError, showSuccess, showInfo } from '@/components/Toast';
import { confirm } from '@/components/ConfirmDialog';
import type { ApiToken } from '@/types';
import styles from './ApiTokens.module.css';

const { Title } = Typography;

function isExpired(dateStr?: string): boolean {
  if (!dateStr) return false;
  return new Date(dateStr).getTime() < Date.now();
}

export default function ApiTokens() {
  const { t } = useTranslation();

  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const [createOpen, setCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [revealToken, setRevealToken] = useState<string | null>(null);

  const [form] = Form.useForm();

  const fetchTokens = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const data = await apiTokens.list();
      setTokens(data);
    } catch (err) {
      setError(true);
      if (err instanceof ApiError) showError(err.message);
      else showError(t('common.error_occurred'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchTokens();
  }, [fetchTokens]);

  const handleCreate = async () => {
    try {
      const values = await form.validate();
      setCreating(true);
      const result = await apiTokens.create({
        name: values.name,
        expires_at: values.expires_at ? new Date(values.expires_at).toISOString() : undefined,
      });
      showSuccess(t('api_tokens.created_success'));
      setCreateOpen(false);
      form.resetFields();
      setRevealToken(result.token);
      fetchTokens();
    } catch (err) {
      if (err instanceof ApiError) showError(err.message);
    } finally {
      setCreating(false);
    }
  };

  const handleRevoke = (token: ApiToken) => {
    confirm({
      title: t('api_tokens.revoke'),
      message: t('api_tokens.revoke_confirm'),
      danger: true,
      confirmText: t('api_tokens.revoke'),
      cancelText: t('common.cancel'),
      onConfirm: async () => {
        try {
          await apiTokens.revoke(token.id);
          showSuccess(t('api_tokens.revoked_success'));
          setTokens((prev) => prev.filter((tk) => tk.id !== token.id));
        } catch (err) {
          if (err instanceof ApiError) showError(err.message);
        }
      },
    });
  };

  const handleCopy = (text: string) => {
    navigator.clipboard.writeText(text).then(() => {
      showInfo(t('common.copied'));
    });
  };

  const columns = [
    {
      title: t('api_tokens.token_name'),
      dataIndex: 'name',
      key: 'name',
      render: (val: string) => <span style={{ fontWeight: 500 }}>{val}</span>,
    },
    {
      title: t('api_tokens.preview'),
      dataIndex: 'token_preview',
      key: 'token_preview',
      render: (val: string) => <span className={styles.tokenCell}>{val}</span>,
    },
    {
      title: t('api_tokens.created'),
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (val: string) => new Date(val).toLocaleDateString(),
    },
    {
      title: t('api_tokens.last_used'),
      dataIndex: 'last_used_at',
      key: 'last_used_at',
      width: 160,
      render: (val?: string) => val ? new Date(val).toLocaleDateString() : '—',
    },
    {
      title: t('api_tokens.expires_at'),
      dataIndex: 'expires_at',
      key: 'expires_at',
      width: 160,
      render: (val?: string) => {
        if (!val) return <Tag size="small">{t('api_tokens.never')}</Tag>;
        const exp = isExpired(val);
        return (
          <span style={{ color: exp ? 'var(--color-danger)' : 'inherit' }}>
            {new Date(val).toLocaleDateString()}
          </span>
        );
      },
    },
    {
      title: t('common.actions'),
      key: 'actions',
      width: 80,
      render: (_: unknown, record: ApiToken) => (
        <Button
          type="text"
          size="mini"
          status="danger"
          icon={<Trash2 size={14} />}
          onClick={() => handleRevoke(record)}
          disabled={isExpired(record.expires_at)}
        />
      ),
    },
  ];

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
          <Button onClick={fetchTokens}>{t('common.try_again')}</Button>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <Title heading={5} style={{ margin: 0 }}>{t('api_tokens.title')}</Title>
        </div>
        <Button type="primary" icon={<Plus size={16} />} onClick={() => setCreateOpen(true)}>
          {t('api_tokens.create')}
        </Button>
      </div>

      {tokens.length === 0 ? (
        <div className={styles.emptyState}>
          <div className={styles.emptyIcon}><Key size={32} /></div>
          <div className={styles.emptyTitle}>{t('api_tokens.no_tokens')}</div>
        </div>
      ) : (
        <div className={styles.tableWrap}>
          <Table
            columns={columns}
            data={tokens}
            rowKey="id"
            pagination={false}
            border={false}
            borderCell={false}
            rowClassName={(record) => (isExpired(record.expires_at) ? styles.expiredRow : '')}
          />
        </div>
      )}

      <Modal
        title={t('api_tokens.create')}
        visible={createOpen}
        onCancel={() => setCreateOpen(false)}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem' }}>
            <Button onClick={() => setCreateOpen(false)}>{t('common.cancel')}</Button>
            <Button type="primary" loading={creating} onClick={handleCreate}>
              {t('common.create')}
            </Button>
          </div>
        }
        style={{ maxWidth: 480 }}
      >
        <Form form={form} layout="vertical">
          <Form.Item label={t('api_tokens.token_name')} field="name" rules={[{ required: true }]}>
            <Input placeholder="e.g. CI/CD Pipeline" />
          </Form.Item>
          <Form.Item label={t('api_tokens.expires_at')} field="expires_at">
            <DatePicker showTime style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={t('api_tokens.created_success')}
        visible={Boolean(revealToken)}
        onCancel={() => setRevealToken(null)}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem' }}>
            <Button
              icon={<Copy size={14} />}
              onClick={() => revealToken && handleCopy(revealToken)}
            >
              {t('api_tokens.copy_token')}
            </Button>
            <Button type="primary" onClick={() => setRevealToken(null)}>
              {t('common.close')}
            </Button>
          </div>
        }
        style={{ maxWidth: 560 }}
      >
        <div className={styles.tokenModal}>
          <div className={styles.tokenWarning}>
            <AlertTriangle size={18} className={styles.tokenWarningIcon} />
            <span>{t('api_tokens.token_created_warning')}</span>
          </div>
          <div className={styles.tokenBox}>
            {revealToken}
          </div>
        </div>
      </Modal>
    </div>
  );
}

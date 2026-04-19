import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Card, Table, Button, Spin, Typography } from '@arco-design/web-react';
import { Plus, Pencil, Trash2, Layers } from 'lucide-react';
import { contentTypes } from '@/api/endpoints';
import { ApiError } from '@/api/client';
import { showError, showSuccess } from '@/components/Toast';
import { confirm } from '@/components/ConfirmDialog';
import type { ContentType } from '@/types';
import styles from './ContentTypeList.module.css';

const { Title } = Typography;

export default function ContentTypeList() {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const [types, setTypes] = useState<ContentType[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const fetchTypes = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const data = await contentTypes.list();
      setTypes(data);
    } catch (err) {
      setError(true);
      if (err instanceof ApiError) {
        showError(err.message);
      } else {
        showError(t('common.error_occurred'));
      }
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchTypes();
  }, [fetchTypes]);

  const handleDelete = (ct: ContentType) => {
    confirm({
      title: t('common.delete'),
      message: t('content_type.delete_confirm'),
      danger: true,
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      onConfirm: async () => {
        try {
          await contentTypes.delete(ct.name);
          showSuccess(t('content_type.deleted_success'));
          setTypes((prev) => prev.filter((item) => item.name !== ct.name));
        } catch (err) {
          if (err instanceof ApiError) {
            showError(err.message);
          } else {
            showError(t('common.error_occurred'));
          }
        }
      },
    });
  };

  if (loading) {
    return (
      <div className={styles.page}>
        <div className={styles.loadingWrap}>
          <Spin size={40} />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className={styles.page}>
        <div className={styles.emptyState}>
          <p>{t('common.error_occurred')}</p>
          <Button onClick={fetchTypes} style={{ marginTop: 16 }}>
            {t('common.try_again')}
          </Button>
        </div>
      </div>
    );
  }

  if (types.length === 0) {
    return (
      <div className={styles.page}>
        <div className={styles.header}>
          <div className={styles.headerLeft}>
            <Title heading={5}>{t('content_type.title')}</Title>
          </div>
          <Button
            type="primary"
            icon={<Plus size={16} />}
            onClick={() => navigate('/admin/content-types/new')}
          >
            {t('content_type.create')}
          </Button>
        </div>
        <Card className={styles.tableCard}>
          <div className={styles.emptyState}>
            <div className={styles.emptyIcon}>
              <Layers size={32} />
            </div>
            <div className={styles.emptyTitle}>{t('content_type.no_types')}</div>
            <div className={styles.emptyDesc}>{t('content_type.no_types_message')}</div>
            <Button
              type="primary"
              icon={<Plus size={16} />}
              onClick={() => navigate('/admin/content-types/new')}
            >
              {t('content_type.create')}
            </Button>
          </div>
        </Card>
      </div>
    );
  }

  const columns = [
    {
      title: t('common.name'),
      dataIndex: 'name',
      key: 'name',
      render: (_: string, record: ContentType) => (
        <div className={styles.nameCell}>
          <span className={styles.displayName}>{record.display_name}</span>
          <span className={styles.apiName}>{record.name}</span>
        </div>
      ),
    },
    {
      title: t('common.description'),
      dataIndex: 'description',
      key: 'description',
      render: (val: string) => (
        <span className={styles.descCell}>{val || '—'}</span>
      ),
    },
    {
      title: t('content_type.fields_section'),
      key: 'fields',
      width: 100,
      render: (_: unknown, record: ContentType) => (
        <span className={styles.fieldCount}>{record.fields?.length ?? 0}</span>
      ),
    },
    {
      title: t('common.actions'),
      key: 'actions',
      width: 120,
      render: (_: unknown, record: ContentType) => (
        <div style={{ display: 'flex', gap: 4 }}>
          <button
            type="button"
            className={styles.actionBtn}
            onClick={() => navigate(`/admin/content-types/${record.name}`)}
            title={t('common.edit')}
          >
            <Pencil size={15} />
          </button>
          <button
            type="button"
            className={`${styles.actionBtn} ${styles.danger}`}
            onClick={() => handleDelete(record)}
            title={t('common.delete')}
          >
            <Trash2 size={15} />
          </button>
        </div>
      ),
    },
  ];

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <Title heading={5}>{t('content_type.title')}</Title>
          <p>{types.length} {types.length === 1 ? 'type' : 'types'}</p>
        </div>
        <Button
          type="primary"
          icon={<Plus size={16} />}
          onClick={() => navigate('/admin/content-types/new')}
        >
          {t('content_type.create')}
        </Button>
      </div>

      <Card className={styles.tableCard}>
        <Table
          columns={columns}
          data={types}
          rowKey="name"
          pagination={false}
          border={false}
          borderCell={false}
        />
      </Card>
    </div>
  );
}

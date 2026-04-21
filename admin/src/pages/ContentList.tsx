import { useEffect, useState, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { Card, Table, Button, Input, Select, Tag, Space } from '@arco-design/web-react';
import {
  Plus,
  Search,
  Trash2,
  Inbox,
  ChevronRight,
  ArrowUpDown,
} from 'lucide-react';
import { content, contentTypes } from '@/api/endpoints';
import type { ContentItem, ContentType, ListParams, PaginatedResponse } from '@/types';
import { showSuccess, showError } from '@/components/Toast';
import { confirm } from '@/components/ConfirmDialog';
import styles from './ContentList.module.css';

const { Option } = Select;

export default function ContentList() {
  const { t } = useTranslation();
  const { contentType } = useParams<{ contentType: string }>();
  const navigate = useNavigate();

  const [ctDef, setCtDef] = useState<ContentType | null>(null);
  const [items, setItems] = useState<ContentItem[]>([]);
  const [pagination, setPagination] = useState({ total: 0, page: 1, perPage: 20 });
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [sortBy, setSortBy] = useState<string>('created_at');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('desc');

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fetchContentType = useCallback(async () => {
    if (!contentType) return;
    try {
      const ct = await contentTypes.get(contentType);
      setCtDef(ct);
    } catch {
      showError(t('common.error_occurred'));
    }
  }, [contentType, t]);

  const fetchItems = useCallback(async () => {
    if (!contentType) return;
    try {
      setLoading(true);
      const params: ListParams = {
        page: pagination.page,
        per_page: pagination.perPage,
        sort: sortBy,
        order: sortOrder,
      };
      if (searchTerm.trim()) {
        params.search = searchTerm.trim();
      }
      if (statusFilter !== 'all') {
        params.filter = { status: statusFilter };
      }
      const res: PaginatedResponse<ContentItem> = await content.list(contentType, params);
      // Backend may return data: null when there are no items
      setItems(res.data ?? []);
      setPagination(prev => ({
        ...prev,
        total: res.meta?.total ?? 0,
      }));
    } catch {
      showError(t('common.error_occurred'));
    } finally {
      setLoading(false);
    }
  }, [contentType, pagination.page, pagination.perPage, sortBy, sortOrder, searchTerm, statusFilter, t]);

  useEffect(() => {
    fetchContentType();
  }, [fetchContentType]);

  useEffect(() => {
    fetchItems();
  }, [fetchItems]);

  const handleSearch = (val: string) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      setSearchTerm(val);
      setPagination(prev => ({ ...prev, page: 1 }));
    }, 300);
  };

  const handleStatusFilter = (val: string) => {
    setStatusFilter(val);
    setPagination(prev => ({ ...prev, page: 1 }));
  };

  const handleSort = (column: string) => {
    if (sortBy === column) {
      setSortOrder(prev => (prev === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortBy(column);
      setSortOrder('desc');
    }
  };

  const handleDelete = (item: ContentItem) => {
    confirm({
      title: t('common.delete'),
      message: t('content.delete_confirm'),
      onConfirm: async () => {
        if (!contentType) return;
        try {
          await content.delete(contentType, item.id);
          showSuccess(t('content.deleted_success'));
          fetchItems();
        } catch {
          showError(t('common.error_occurred'));
        }
      },
      danger: true,
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
    });
  };

  const handlePageChange = (page: number, perPage: number) => {
    setPagination({ ...pagination, page, perPage });
  };

  // fields may be null from the API even when ctDef is set — use ?. on both ctDef and fields
  const titleField = ctDef?.fields?.find(f => f.type === 'text' || f.type === 'slug')?.name || 'title';

  const getTitle = (item: ContentItem): string => {
    const val = item[titleField];
    if (typeof val === 'string' && val.length > 0) return val;
    if (item.title && typeof item.title === 'string') return item.title;
    return item.id.slice(0, 8);
  };

  const getStatusTag = (status: string) => {
    if (status === 'published') {
      return <Tag color="green" size="small">{t('content.status_published')}</Tag>;
    }
    return <Tag color="orange" size="small">{t('content.status_draft')}</Tag>;
  };

  const formatDate = (dateStr: string): string => {
    return new Date(dateStr).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  };

  const columns = [
    {
      title: (
        <button type="button" className={styles.sortHeader} onClick={() => handleSort(titleField)}>
          {titleField} <ArrowUpDown size={12} />
        </button>
      ),
      dataIndex: 'data',
      key: 'title',
      width: 300,
      render: (_: unknown, record: ContentItem) => (
        <span className={styles.titleCell}>{getTitle(record)}</span>
      ),
    },
    {
      title: t('common.status'),
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (status: string) => getStatusTag(status),
    },
    {
      title: (
        <button type="button" className={styles.sortHeader} onClick={() => handleSort('created_at')}>
          {t('common.created_at')} <ArrowUpDown size={12} />
        </button>
      ),
      dataIndex: 'created_at',
      key: 'created_at',
      width: 140,
      render: (val: string) => <span className={styles.dateCell}>{formatDate(val)}</span>,
    },
    {
      title: (
        <button type="button" className={styles.sortHeader} onClick={() => handleSort('updated_at')}>
          {t('common.updated_at')} <ArrowUpDown size={12} />
        </button>
      ),
      dataIndex: 'updated_at',
      key: 'updated_at',
      width: 140,
      render: (val: string) => <span className={styles.dateCell}>{formatDate(val)}</span>,
    },
    {
      title: t('common.actions'),
      key: 'actions',
      width: 80,
      render: (_: unknown, record: ContentItem) => (
        <div className={styles.actionCell}>
          <button
            type="button"
            className={`${styles.actionBtn} ${styles.actionBtnDanger}`}
            onClick={(e) => {
              e.stopPropagation();
              handleDelete(record);
            }}
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
      <div className={styles.breadcrumb}>
        <Link to="/admin/content" className={styles.breadcrumbLink}>{t('nav.content')}</Link>
        <ChevronRight size={14} className={styles.breadcrumbSep} />
        <span className={styles.breadcrumbCurrent}>{ctDef?.display_name || contentType}</span>
      </div>

      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <h2 className={styles.headerTitle}>
            {t('content.list', { type: ctDef?.display_name || contentType })}
          </h2>
        </div>
        <Link to={`/admin/content/${contentType}/new`}>
          <Button type="primary" icon={<Plus size={16} />}>
            {t('common.create')}
          </Button>
        </Link>
      </div>

      <div className={styles.toolbar}>
        <Input
          className={styles.searchBox}
          prefix={<Search size={16} />}
          placeholder={t('common.search') + '...'}
          onChange={handleSearch}
          allowClear
        />
        <Select
          className={styles.filterSelect}
          value={statusFilter}
          onChange={handleStatusFilter}
        >
          <Option value="all">{t('common.all')}</Option>
          <Option value="draft">{t('content.status_draft')}</Option>
          <Option value="published">{t('content.status_published')}</Option>
        </Select>
      </div>

      <Card className={styles.tableCard} bordered={false}>
        {loading ? (
          <div className={`skeleton ${styles.skeletonTable}`} />
        ) : items.length === 0 ? (
          <div className={styles.emptyState}>
            <Inbox size={48} className={styles.emptyIcon} />
            <div className={styles.emptyTitle}>{t('content.no_content')}</div>
            <div className={styles.emptyDesc}>
              {t('content.no_content_message', { type: ctDef?.display_name || contentType })}
            </div>
            <Link to={`/admin/content/${contentType}/new`}>
              <Button type="primary" icon={<Plus size={16} />}>
                {t('common.create')}
              </Button>
            </Link>
          </div>
        ) : (
          <>
            <Table
              columns={columns}
              data={items}
              rowKey="id"
              pagination={false}
              onRow={(record: ContentItem) => ({
                onClick: () => navigate(`/admin/content/${contentType}/${record.id}`),
              })}
              className={styles.clickableRow}
            />
            <div className={styles.paginationWrap}>
              <Space>
                <span style={{ fontSize: '0.8125rem', color: 'var(--color-text-tertiary)' }}>
                  {t('common.showing', {
                    from: (pagination.page - 1) * pagination.perPage + 1,
                    to: Math.min(pagination.page * pagination.perPage, pagination.total),
                    total: pagination.total,
                  })}
                </span>
                <Button
                  size="small"
                  disabled={pagination.page <= 1}
                  onClick={() => handlePageChange(pagination.page - 1, pagination.perPage)}
                >
                  {t('common.previous')}
                </Button>
                <Button
                  size="small"
                  disabled={pagination.page * pagination.perPage >= pagination.total}
                  onClick={() => handlePageChange(pagination.page + 1, pagination.perPage)}
                >
                  {t('common.next')}
                </Button>
              </Space>
            </div>
          </>
        )}
      </Card>
    </div>
  );
}

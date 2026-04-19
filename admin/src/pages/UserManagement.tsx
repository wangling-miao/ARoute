import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button, Table, Spin, Typography, Tag, Input,
  Drawer, Form, Select,
} from '@arco-design/web-react';
import { Plus, Search, UserPlus, Trash2, Pencil, Users } from 'lucide-react';
import { users, roles as rolesApi } from '@/api/endpoints';
import { ApiError } from '@/api/client';
import { showError, showSuccess } from '@/components/Toast';
import { confirm } from '@/components/ConfirmDialog';
import { useAuth } from '@/contexts/AuthContext';
import type { User, Role } from '@/types';
import styles from './UserManagement.module.css';

const { Title } = Typography;

const STATUS_COLORS: Record<string, string> = {
  active: 'green',
  inactive: 'gray',
  suspended: 'orange',
};

export default function UserManagement() {
  const { t } = useTranslation();
  const { user: currentUser } = useAuth();

  const [data, setData] = useState<User[]>([]);
  const [allRoles, setAllRoles] = useState<Role[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [search, setSearch] = useState('');

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [saving, setSaving] = useState(false);

  const [form] = Form.useForm();
  const perPage = 15;

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const params: Record<string, unknown> = { page, per_page: perPage };
      if (search.trim()) params.search = search.trim();
      const res = await users.list(params);
      setData(res.data);
      setTotal(res.meta.total);
    } catch (err) {
      setError(true);
      if (err instanceof ApiError) showError(err.message);
      else showError(t('common.error_occurred'));
    } finally {
      setLoading(false);
    }
  }, [page, search, t]);

  const fetchRoles = useCallback(async () => {
    try {
      const r = await rolesApi.list();
      setAllRoles(r);
    } catch { /* roles fetch failure is non-critical */ }
  }, []);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  useEffect(() => {
    fetchRoles();
  }, [fetchRoles]);

  const openCreate = () => {
    setEditingUser(null);
    form.resetFields();
    form.setFieldValue('status', 'active');
    form.setFieldValue('roles', []);
    setDrawerOpen(true);
  };

  const openEdit = (user: User) => {
    setEditingUser(user);
    form.setFieldValue('username', user.username);
    form.setFieldValue('email', user.email);
    form.setFieldValue('roles', user.roles);
    form.setFieldValue('status', user.status);
    form.setFieldValue('password', '');
    setDrawerOpen(true);
  };

  const handleSave = async () => {
    try {
      const values = await form.validate();
      setSaving(true);

      if (editingUser) {
        const updateData: Record<string, unknown> = {
          username: values.username,
          email: values.email,
          roles: values.roles,
          status: values.status,
        };
        if (values.password) {
          updateData.password = values.password;
        }
        await users.update(editingUser.id, updateData);
      } else {
        await users.create({
          username: values.username,
          email: values.email,
          password: values.password,
          roles: values.roles || [],
        });
      }

      showSuccess(t('users.saved_success'));
      setDrawerOpen(false);
      fetchData();
    } catch (err) {
      if (err instanceof ApiError) showError(err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = (user: User) => {
    if (user.id === currentUser?.id) {
      showError('Cannot delete your own account');
      return;
    }
    confirm({
      title: t('common.delete'),
      message: t('users.delete_confirm'),
      danger: true,
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      onConfirm: async () => {
        try {
          await users.delete(user.id);
          showSuccess(t('users.deleted_success'));
          setData((prev) => prev.filter((u) => u.id !== user.id));
          setTotal((prev) => prev - 1);
        } catch (err) {
          if (err instanceof ApiError) showError(err.message);
        }
      },
    });
  };

  const totalPages = Math.ceil(total / perPage);

  const columns = [
    {
      title: t('users.username'),
      key: 'user',
      render: (_: unknown, record: User) => (
        <div className={styles.userCell}>
          <span className={styles.username}>{record.username}</span>
          <span className={styles.userEmail}>{record.email}</span>
        </div>
      ),
    },
    {
      title: t('users.role'),
      key: 'roles',
      render: (_: unknown, record: User) => (
        <span>
          {record.roles.map((r) => (
            <Tag key={r} size="small" className={styles.roleTag}>{r}</Tag>
          ))}
        </span>
      ),
    },
    {
      title: t('users.status'),
      dataIndex: 'status',
      key: 'status',
      width: 110,
      render: (val: string) => (
        <Tag color={STATUS_COLORS[val] ?? 'gray'} size="small">
          {val}
        </Tag>
      ),
    },
    {
      title: t('common.created_at'),
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (val: string) => new Date(val).toLocaleDateString(),
    },
    {
      title: t('common.actions'),
      key: 'actions',
      width: 100,
      render: (_: unknown, record: User) => (
        <div style={{ display: 'flex', gap: 4 }}>
          <Button
            type="text"
            size="mini"
            icon={<Pencil size={14} />}
            onClick={() => openEdit(record)}
          />
          <Button
            type="text"
            size="mini"
            status="danger"
            icon={<Trash2 size={14} />}
            onClick={() => handleDelete(record)}
            disabled={record.id === currentUser?.id}
          />
        </div>
      ),
    },
  ];

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <Title heading={5} style={{ margin: 0 }}>{t('users.title')}</Title>
        </div>
        <Button type="primary" icon={<Plus size={16} />} onClick={openCreate}>
          {t('users.create')}
        </Button>
      </div>

      <div className={styles.toolbar}>
        <Input
          prefix={<Search size={16} />}
          placeholder={`${t('common.search')}...`}
          value={search}
          onChange={setSearch}
          allowClear
          style={{ maxWidth: 280 }}
          onClear={() => { setSearch(''); setPage(1); }}
          onKeyDown={(e) => { if (e.key === 'Enter') { setPage(1); fetchData(); } }}
        />
      </div>

      {loading ? (
        <div className={styles.loadingWrap}><Spin size={40} /></div>
      ) : error ? (
        <div className={styles.emptyState}>
          <p>{t('common.error_occurred')}</p>
          <Button onClick={fetchData}>{t('common.try_again')}</Button>
        </div>
      ) : data.length === 0 ? (
        <div className={styles.emptyState}>
          <div className={styles.emptyIcon}><Users size={32} /></div>
          <div className={styles.emptyTitle}>{t('users.no_users')}</div>
          <Button type="primary" icon={<UserPlus size={16} />} onClick={openCreate} style={{ marginTop: 16 }}>
            {t('users.create')}
          </Button>
        </div>
      ) : (
        <>
          <div className={styles.tableWrap}>
            <Table
              columns={columns}
              data={data}
              rowKey="id"
              pagination={false}
              border={false}
              borderCell={false}
            />
          </div>
          {totalPages > 1 && (
            <div className={styles.paginationWrap}>
              <Button disabled={page <= 1} onClick={() => setPage((p) => p - 1)} size="small">
                {t('common.previous')}
              </Button>
              <span style={{ margin: '0 0.75rem', fontSize: '0.8125rem', color: 'var(--color-text-tertiary)' }}>
                {page} / {totalPages}
              </span>
              <Button disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)} size="small">
                {t('common.next')}
              </Button>
            </div>
          )}
        </>
      )}

      <Drawer
        title={editingUser ? t('users.edit') : t('users.create')}
        visible={drawerOpen}
        onCancel={() => setDrawerOpen(false)}
        footer={
          <div className={styles.drawerFooter}>
            <Button onClick={() => setDrawerOpen(false)}>{t('common.cancel')}</Button>
            <Button type="primary" loading={saving} onClick={handleSave}>{t('common.save')}</Button>
          </div>
        }
        width={480}
      >
        <Form form={form} layout="vertical">
          <Form.Item label={t('users.username')} field="username" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item label={t('users.email')} field="email" rules={[
            { required: true },
            { type: 'email' },
          ]}>
            <Input />
          </Form.Item>
          <Form.Item
            label={t('users.password')}
            field="password"
            rules={editingUser ? [] : [{ required: true }]}
          >
            <Input.Password placeholder={editingUser ? t('users.password_change') : ''} />
          </Form.Item>
          <Form.Item label={t('users.assign_roles')} field="roles">
            <Select mode="multiple" placeholder={t('users.assign_roles')}>
              {allRoles.map((r) => (
                <Select.Option key={r.name} value={r.name}>{r.name}</Select.Option>
              ))}
            </Select>
          </Form.Item>
          {editingUser && (
            <Form.Item label={t('users.status')} field="status">
              <Select>
                <Select.Option value="active">Active</Select.Option>
                <Select.Option value="inactive">Inactive</Select.Option>
                <Select.Option value="suspended">Suspended</Select.Option>
              </Select>
            </Form.Item>
          )}
        </Form>
      </Drawer>
    </div>
  );
}

import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Spin, Typography, Switch, Input } from '@arco-design/web-react';
import { ChevronDown, ChevronRight, Save } from 'lucide-react';
import { roles as rolesApi } from '@/api/endpoints';
import { ApiError } from '@/api/client';
import { showError, showSuccess } from '@/components/Toast';
import { usePermissions } from '@/hooks/usePermissions';
import type { Role, Permission } from '@/types';
import styles from './RoleManagement.module.css';

const { Title } = Typography;

const RESOURCES = ['content_types', 'content', 'media', 'users', 'roles', 'plugins', 'settings', 'api_tokens', 'menus'];
const ACTIONS = ['create', 'read', 'update', 'delete'];

const BUILT_IN_ROLES = ['admin', 'public'];

export default function RoleManagement() {
  const { t } = useTranslation();
  const { can, isAdmin } = usePermissions();
  const canEdit = can('roles', 'update');

  const [roles, setRoles] = useState<Role[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [editedPerms, setEditedPerms] = useState<Record<string, Permission[]>>({});
  const [editedDesc, setEditedDesc] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState<string | null>(null);

  const fetchRoles = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const data = await rolesApi.list();
      setRoles(data);
    } catch (err) {
      setError(true);
      if (err instanceof ApiError) showError(err.message);
      else showError(t('common.error_occurred'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchRoles();
  }, [fetchRoles]);

  const getPerms = (role: Role): Permission[] => {
    return editedPerms[role.id] ?? (role.permissions ?? []);
  };

  const getDesc = (role: Role): string => {
    return editedDesc[role.id] ?? role.description;
  };

  const toggleExpand = (role: Role) => {
    if (expandedId === role.id) {
      setExpandedId(null);
    } else {
      setExpandedId(role.id);
      if (!editedPerms[role.id]) {
        setEditedPerms((prev) => ({ ...prev, [role.id]: [...(role.permissions ?? [])] }));
        setEditedDesc((prev) => ({ ...prev, [role.id]: role.description }));
      }
    }
  };

  const isActionAllowed = (perms: Permission[], resource: string, action: string): boolean => {
    const wildcard = perms.find((p) => p.resource === '*');
    if (wildcard && (wildcard.actions.includes('*') || wildcard.actions.includes(action))) {
      return true;
    }
    const perm = perms.find((p) => p.resource === resource);
    if (!perm) return false;
    return perm.actions.includes('*') || perm.actions.includes(action);
  };

  const toggleAction = (roleId: string, resource: string, action: string) => {
    setEditedPerms((prev) => {
      const perms = [...(prev[roleId] ?? [])];
      const idx = perms.findIndex((p) => p.resource === resource);
      if (idx >= 0) {
        const actions = perms[idx].actions.includes(action)
          ? perms[idx].actions.filter((a) => a !== action)
          : [...perms[idx].actions, action];
        if (actions.length === 0) {
          perms.splice(idx, 1);
        } else {
          perms[idx] = { ...perms[idx], actions };
        }
      } else {
        perms.push({ resource, actions: [action] });
      }
      return { ...prev, [roleId]: perms };
    });
  };

  const selectAllForResource = (roleId: string, resource: string, checked: boolean) => {
    setEditedPerms((prev) => {
      const perms = [...(prev[roleId] ?? [])];
      const idx = perms.findIndex((p) => p.resource === resource);
      if (checked) {
        if (idx >= 0) {
          perms[idx] = { ...perms[idx], actions: [...ACTIONS] };
        } else {
          perms.push({ resource, actions: [...ACTIONS] });
        }
      } else {
        if (idx >= 0) perms.splice(idx, 1);
      }
      return { ...prev, [roleId]: perms };
    });
  };

  const handleSave = async (role: Role) => {
    setSaving(role.id);
    try {
      await rolesApi.update(role.id, {
        description: getDesc(role),
        permissions: getPerms(role),
      });
      showSuccess(t('roles.saved_success'));
      setRoles((prev) =>
        prev.map((r) =>
          r.id === role.id
            ? { ...r, description: getDesc(role), permissions: getPerms(role) }
            : r,
        ),
      );
      setEditedPerms((prev) => {
        const next = { ...prev };
        delete next[role.id];
        return next;
      });
      setEditedDesc((prev) => {
        const next = { ...prev };
        delete next[role.id];
        return next;
      });
      setExpandedId(null);
    } catch (err) {
      if (err instanceof ApiError) showError(err.message);
    } finally {
      setSaving(null);
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
        <div style={{ textAlign: 'center', padding: '4rem 2rem' }}>
          <p>{t('common.error_occurred')}</p>
          <Button onClick={fetchRoles}>{t('common.try_again')}</Button>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <Title heading={5} style={{ margin: 0 }}>{t('roles.title')}</Title>
        </div>
      </div>

      <div className={styles.roleList}>
        {roles.map((role) => {
          const isExpanded = expandedId === role.id;
          const isBuiltIn = BUILT_IN_ROLES.includes(role.name.toLowerCase());
          const isEditable = canEdit && (isAdmin || role.name !== 'admin');
          const perms = getPerms(role);
          const totalPermCount = perms.reduce((sum, p) => sum + p.actions.length, 0);

          return (
            <div
              key={role.id}
              className={`${styles.roleCard} ${isExpanded ? styles.roleCardExpanded : ''}`}
            >
              <button
                type="button"
                className={styles.roleHeader}
                onClick={() => toggleExpand(role)}
              >
                {isExpanded ? <ChevronDown size={18} /> : <ChevronRight size={18} />}
                <div className={styles.roleInfo}>
                  <div className={styles.roleName}>{role.name}</div>
                  <div className={styles.roleDesc}>{role.description}</div>
                </div>
                <div className={styles.roleMeta}>
                  {isBuiltIn && <span className={styles.builtInTag}>Built-in</span>}
                  <span className={styles.permCount}>
                    {totalPermCount} {t('roles.permissions').toLowerCase()}
                  </span>
                </div>
              </button>

              {isExpanded && (
                <div className={styles.roleBody}>
                  <div style={{ marginBottom: '1rem' }}>
                    <Input
                      value={getDesc(role)}
                      onChange={(v) => setEditedDesc((prev) => ({ ...prev, [role.id]: v }))}
                      placeholder={t('common.description')}
                      disabled={!isEditable}
                    />
                  </div>

                  <table className={styles.permMatrix}>
                    <thead>
                      <tr>
                        <th>{t('roles.resource')}</th>
                        {ACTIONS.map((action) => (
                          <th key={action}>{action}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {RESOURCES.map((resource) => {
                        const allChecked = ACTIONS.every((a) => isActionAllowed(perms, resource, a));
                        return (
                          <tr key={resource}>
                            <td>
                              <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                                <Switch
                                  size="small"
                                  checked={allChecked}
                                  onChange={(v) => selectAllForResource(role.id, resource, v)}
                                  disabled={!isEditable}
                                />
                                <span className={styles.selectAllLabel}>{resource}</span>
                              </div>
                            </td>
                            {ACTIONS.map((action) => (
                              <td key={action}>
                                <Switch
                                  size="small"
                                  checked={isActionAllowed(perms, resource, action)}
                                  onChange={() => toggleAction(role.id, resource, action)}
                                  disabled={!isEditable}
                                />
                              </td>
                            ))}
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>

                  {isEditable && (
                  <div className={styles.footer}>
                    <Button onClick={() => setExpandedId(null)}>
                      {t('common.cancel')}
                    </Button>
                    <Button
                      type="primary"
                      icon={<Save size={16} />}
                      loading={saving === role.id}
                      onClick={() => handleSave(role)}
                    >
                      {t('common.save')}
                    </Button>
                  </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

import { useEffect, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card, Button, Input, Select, Switch, Modal, Form, Tag, Spin,
} from '@arco-design/web-react';
import {
  Plus, Trash2, GripVertical, Edit3, ExternalLink, CornerDownRight,
} from 'lucide-react';
import {
  DndContext, PointerSensor, useSensor, useSensors,
  type DragEndEvent, type DragStartEvent, type DragOverEvent, closestCenter,
} from '@dnd-kit/core';
import {
  SortableContext, verticalListSortingStrategy, useSortable,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { content } from '@/api/endpoints';
import type { ContentItem } from '@/types';
import { showSuccess, showError } from '@/components/Toast';
import { confirm } from '@/components/ConfirmDialog';
import styles from './MenuManagement.module.css';

const { Option } = Select;
const INDENT = 28;

interface FlatItem {
  id: string;
  title: string;
  slug: string;
  menu_type: string;
  url: string;
  target_id: string;
  parentId: string;
  sort_order: number;
  is_active: boolean;
  depth: number;
}

const MENU_TYPES = ['custom_page', 'post', 'custom_link', 'tag', 'category'];

function buildFlatList(raw: MenuItemRaw[]): FlatItem[] {
  const map = new Map<string, MenuItemRaw>();
  for (const r of raw) map.set(r.id, r);

  const visited = new Set<string>();
  const result: FlatItem[] = [];

  function walk(id: string, depth: number) {
    if (visited.has(id)) return;
    visited.add(id);
    const item = map.get(id);
    if (!item) return;
    result.push({
      id: item.id, title: item.title, slug: item.slug, menu_type: item.menu_type,
      url: item.url, target_id: item.target_id, parentId: item.parent,
      sort_order: item.sort_order, is_active: item.is_active, depth,
    });
    const children = raw.filter(r => r.parent === id).sort((a, b) => a.sort_order - b.sort_order);
    for (const child of children) walk(child.id, depth + 1);
  }

  const roots = raw.filter(r => !r.parent || !map.has(r.parent)).sort((a, b) => a.sort_order - b.sort_order);
  for (const root of roots) walk(root.id, 0);
  for (const r of raw) {
    if (!visited.has(r.id)) {
      result.push({
        id: r.id, title: r.title, slug: r.slug, menu_type: r.menu_type,
        url: r.url, target_id: r.target_id, parentId: '', sort_order: r.sort_order,
        is_active: r.is_active, depth: 0,
      });
    }
  }
  return result;
}

interface MenuItemRaw {
  id: string;
  title: string;
  slug: string;
  menu_type: string;
  url: string;
  target_id: string;
  parent: string;
  sort_order: number;
  is_active: boolean;
}

function SortableMenuItem({
  item, indentHint, onEdit, onDelete, onToggle,
}: {
  item: FlatItem;
  indentHint: number;
  onEdit: (item: FlatItem) => void;
  onDelete: (item: FlatItem) => void;
  onToggle: (item: FlatItem, active: boolean) => void;
}) {
  const { t } = useTranslation();
  const {
    attributes, listeners, setNodeRef, transform, transition, isDragging,
  } = useSortable({ id: item.id });

  const visualDepth = item.depth + indentHint;

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.4 : 1,
    paddingLeft: visualDepth * INDENT + 12,
  };

  const typeLabel = t(`menus.type_${item.menu_type}`, item.menu_type);

  return (
    <div ref={setNodeRef} style={style} className={styles.menuItem}>
      <span className={styles.dragHandle} {...attributes} {...listeners}>
        <GripVertical size={16} />
      </span>
      {visualDepth > 0 && <CornerDownRight size={14} className={styles.childIndicator} />}
      <span className={styles.itemTitle}>{item.title || t('common.no_data')}</span>
      <Tag size="small" style={{ margin: '0 8px' }}>{typeLabel}</Tag>
      {!item.is_active && <Tag color="red" size="small">{t('menus.inactive')}</Tag>}
      {item.url && (
        <span className={styles.itemUrl} title={item.url}>
          <ExternalLink size={12} /> {item.url}
        </span>
      )}
      <span className={styles.itemActions}>
        <button type="button" className={styles.iconBtn} onClick={() => onEdit(item)} title={t('common.edit')}>
          <Edit3 size={14} />
        </button>
        <button type="button" className={styles.iconBtnDanger} onClick={() => onDelete(item)} title={t('common.delete')}>
          <Trash2 size={14} />
        </button>
        <Switch size="small" checked={item.is_active} onChange={(v: boolean) => onToggle(item, v)} />
      </span>
    </div>
  );
}

export default function MenuManagement() {
  const { t } = useTranslation();
  const [flatItems, setFlatItems] = useState<FlatItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingItem, setEditingItem] = useState<FlatItem | null>(null);
  const [saving, setSaving] = useState(false);
  const [targetOptions, setTargetOptions] = useState<{ id: string; label: string; slug: string }[]>([]);

  // Drag state: track active item and horizontal indent offset
  const [activeId, setActiveId] = useState<string | null>(null);
  const [indentDelta, setIndentDelta] = useState(0);

  const [formTitle, setFormTitle] = useState('');
  const [formMenuType, setFormMenuType] = useState('custom_link');
  const [formUrl, setFormUrl] = useState('');
  const [formTargetId, setFormTargetId] = useState('');
  const [formParent, setFormParent] = useState('');
  const [formIsActive, setFormIsActive] = useState(true);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
  );

  const fetchItems = useCallback(async () => {
    try {
      setLoading(true);
      const res = await content.list('menu', { per_page: 200, sort: 'sort_order', order: 'asc' });
      const data = (res.data ?? []) as ContentItem[];
      const raw: MenuItemRaw[] = data.map(d => ({
        id: d.id,
        title: (d.title as string) || '',
        slug: (d.slug as string) || '',
        menu_type: (d.menu_type as string) || 'custom_link',
        url: (d.url as string) || '',
        target_id: (d.target_id as string) || '',
        parent: (d.parent as string) || '',
        sort_order: typeof d.sort_order === 'number' ? d.sort_order : parseInt(String(d.sort_order || '0'), 10),
        is_active: d.is_active !== false && d.is_active !== 'false',
      }));
      setFlatItems(buildFlatList(raw));
    } catch {
      showError(t('common.error_occurred'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  const fetchTargets = useCallback(async (menuType: string) => {
    if (menuType === 'custom_link') {
      setTargetOptions([]);
      return;
    }
    const contentTypeMap: Record<string, string> = {
      custom_page: 'page', post: 'post', tag: 'tag', category: 'category',
    };
    const ct = contentTypeMap[menuType];
    if (!ct) return;
    try {
      const res = await content.list(ct, { per_page: 200 });
      const data = (res.data ?? []) as ContentItem[];
      setTargetOptions(data.map(d => ({
        id: d.id,
        label: (d.title as string) || (d.name as string) || d.id.slice(0, 8),
        slug: (d.slug as string) || '',
      })));
    } catch {
      setTargetOptions([]);
    }
  }, []);

  useEffect(() => { fetchItems(); }, [fetchItems]);

  const openCreateModal = () => {
    setEditingItem(null);
    setFormTitle('');
    setFormMenuType('custom_link');
    setFormUrl('');
    setFormTargetId('');
    setFormParent('');
    setFormIsActive(true);
    setTargetOptions([]);
    setModalVisible(true);
  };

  const openEditModal = (item: FlatItem) => {
    setEditingItem(item);
    setFormTitle(item.title);
    setFormMenuType(item.menu_type);
    setFormUrl(item.url);
    setFormTargetId(item.target_id);
    setFormParent(item.parentId);
    setFormIsActive(item.is_active);
    fetchTargets(item.menu_type);
    setModalVisible(true);
  };

  const handleSave = async () => {
    if (!formTitle.trim()) {
      showError(t('common.required'));
      return;
    }
    try {
      setSaving(true);
      const maxOrder = flatItems.reduce((max, i) => Math.max(max, i.sort_order), 0);
      const data: Record<string, unknown> = {
        title: formTitle.trim(),
        menu_type: formMenuType,
        url: formUrl.trim(),
        target_id: formTargetId,
        parent: formParent || null,
        sort_order: editingItem ? editingItem.sort_order : maxOrder + 1,
        is_active: formIsActive,
        status: formIsActive ? 'published' : 'draft',
      };
      if (editingItem) {
        await content.update('menu', editingItem.id, data);
      } else {
        await content.create('menu', data);
      }
      showSuccess(t('menus.saved_success'));
      setModalVisible(false);
      fetchItems();
    } catch {
      showError(t('common.error_occurred'));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = (item: FlatItem) => {
    confirm({
      title: t('common.delete'),
      message: t('menus.delete_confirm'),
      onConfirm: async () => {
        try {
          await content.delete('menu', item.id);
          showSuccess(t('menus.deleted_success'));
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

  const handleToggle = async (item: FlatItem, active: boolean) => {
    try {
      await content.update('menu', item.id, {
        is_active: active,
        status: active ? 'published' : 'draft',
      });
      fetchItems();
    } catch {
      showError(t('common.error_occurred'));
    }
  };

  const handleDragStart = (event: DragStartEvent) => {
    setActiveId(String(event.active.id));
    setIndentDelta(0);
  };

  const handleDragOver = (event: DragOverEvent) => {
    if (!activeId) return;
    // Calculate indent from horizontal drag distance
    const dx = event.delta.x;
    const newDelta = Math.max(0, Math.round(dx / INDENT));
    setIndentDelta(newDelta);
  };

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;
    const currentIndentDelta = indentDelta;
    const currentActiveId = activeId;

    setActiveId(null);
    setIndentDelta(0);

    if (!over || active.id === over.id || !currentActiveId) return;

    const aId = String(active.id);
    const oId = String(over.id);
    const aIdx = flatItems.findIndex(i => i.id === aId);
    const oIdx = flatItems.findIndex(i => i.id === oId);
    if (aIdx === -1 || oIdx === -1) return;

    // Build reordered list
    const newItems = [...flatItems];
    const [moved] = newItems.splice(aIdx, 1);
    const insertIdx = aIdx < oIdx ? oIdx - 1 : oIdx;
    newItems.splice(insertIdx, 0, moved);

    // Determine new parent based on indent delta
    // indentDelta > 0 means user dragged right -> make child of the item above
    let newParent = moved.parentId;
    if (currentIndentDelta > 0) {
      // Find the item directly above the insert position -> make it the parent
      const aboveIdx = insertIdx - 1;
      if (aboveIdx >= 0) {
        newParent = newItems[aboveIdx].id;
      }
    } else if (currentIndentDelta < 0) {
      // Dragged left — use the depth of the item above minus 1
      const aboveIdx = insertIdx - 1;
      if (aboveIdx < 0) {
        newParent = '';
      } else {
        const targetDepth = Math.max(0, newItems[aboveIdx].depth - 1);
        if (targetDepth === 0) {
          newParent = '';
        } else {
          // Walk up to find ancestor at targetDepth
          for (let i = aboveIdx; i >= 0; i--) {
            if (newItems[i].depth === targetDepth) {
              newParent = newItems[i].parentId;
              break;
            }
          }
        }
      }
    }

    // Batch update sort_order and parent for the moved item
    const updates: Promise<unknown>[] = [];
    for (let i = 0; i < newItems.length; i++) {
      const item = newItems[i];
      const newOrder = i;
      const needsParentUpdate = item.id === aId;
      if (item.sort_order !== newOrder || needsParentUpdate) {
        const updateData: Record<string, unknown> = { sort_order: newOrder };
        if (needsParentUpdate) updateData.parent = newParent || null;
        updates.push(content.update('menu', item.id, updateData));
      }
    }

    try {
      for (const update of updates) {
        await update;
      }
      fetchItems();
    } catch {
      showError(t('common.error_occurred'));
      fetchItems();
    }
  };

  const handleDragCancel = () => {
    setActiveId(null);
    setIndentDelta(0);
  };

  const parentOptions = flatItems.filter(i => i.id !== editingItem?.id);

  return (
    <div className={styles.page}>
      <Card className={styles.header}>
        <div className={styles.headerInner}>
          <h1 className={styles.pageTitle}>{t('menus.title')}</h1>
          <Button type="primary" icon={<Plus size={16} />} onClick={openCreateModal}>
            {t('menus.add_item')}
          </Button>
        </div>
      </Card>

      <Card className={styles.mainCard}>
        {loading ? (
          <div className={styles.loadingWrap}><Spin /></div>
        ) : flatItems.length === 0 ? (
          <div className={styles.empty}>
            <p>{t('menus.no_items')}</p>
            <p className={styles.emptyHint}>{t('menus.no_items_message')}</p>
          </div>
        ) : (
          <>
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragStart={handleDragStart}
              onDragOver={handleDragOver}
              onDragEnd={handleDragEnd}
              onDragCancel={handleDragCancel}
            >
              <SortableContext items={flatItems.map(i => i.id)} strategy={verticalListSortingStrategy}>
                {flatItems.map(item => (
                  <SortableMenuItem
                    key={item.id}
                    item={item}
                    indentHint={item.id === activeId ? indentDelta : 0}
                    onEdit={openEditModal}
                    onDelete={handleDelete}
                    onToggle={handleToggle}
                  />
                ))}
              </SortableContext>
            </DndContext>
            <p className={styles.dragTip}>{t('menus.drag_hint')}</p>
          </>
        )}
      </Card>

      <Modal
        title={editingItem ? t('menus.edit_item') : t('menus.add_item')}
        visible={modalVisible}
        onOk={handleSave}
        onCancel={() => setModalVisible(false)}
        confirmLoading={saving}
        okText={t('common.save')}
        cancelText={t('common.cancel')}
      >
        <Form layout="vertical">
          <Form.Item label={t('menus.field_title')} required>
            <Input value={formTitle} onChange={setFormTitle} placeholder={t('menus.field_title')} />
          </Form.Item>
          <Form.Item label={t('menus.field_type')}>
            <Select value={formMenuType} onChange={(v: string) => {
              setFormMenuType(v);
              setFormTargetId('');
              setFormUrl('');
              fetchTargets(v);
            }}>
              {MENU_TYPES.map(mt => (
                <Option key={mt} value={mt}>{t(`menus.type_${mt}`, mt)}</Option>
              ))}
            </Select>
          </Form.Item>
          {formMenuType === 'custom_link' ? (
            <Form.Item label={t('menus.field_url')}>
              <Input value={formUrl} onChange={setFormUrl} placeholder={t('menus.url_hint')} />
            </Form.Item>
          ) : (
            <Form.Item label={t('menus.field_target')}>
              <Select
                value={formTargetId || undefined}
                onChange={(v: string) => {
                  setFormTargetId(v);
                  const opt = targetOptions.find(o => o.id === v);
                  if (opt) {
                    const slug = opt.slug || opt.label.toLowerCase().replace(/\s+/g, '-');
                    const urlMap: Record<string, string> = {
                      custom_page: `/page/${slug}`,
                      post: `/posts/${slug}`,
                      tag: `/posts?tag=${slug}`,
                      category: `/posts?category=${slug}`,
                    };
                    setFormUrl(urlMap[formMenuType] || '/');
                  }
                }}
                placeholder={t('menus.select_target')}
                allowClear
              >
                {targetOptions.map(opt => (
                  <Option key={opt.id} value={opt.id}>{opt.label}</Option>
                ))}
              </Select>
            </Form.Item>
          )}
          <Form.Item label={t('menus.field_parent')}>
            <Select
              value={formParent || undefined}
              onChange={setFormParent}
              placeholder={t('menus.select_parent')}
              allowClear
            >
              {parentOptions.map(opt => (
                <Option key={opt.id} value={opt.id}>{'　'.repeat(opt.depth)}{opt.title}</Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item label={t('menus.field_is_active')}>
            <Switch checked={formIsActive} onChange={setFormIsActive} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}

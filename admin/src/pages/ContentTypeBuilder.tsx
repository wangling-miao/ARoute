import { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Button, Input, Select, Switch, InputNumber,
  Card, Spin, Typography, Dropdown, Tag, Menu,
} from '@arco-design/web-react';
import {
  ArrowLeft, Plus, GripVertical, Pencil, Trash2,
  Save, Type, Hash, ToggleLeft, Calendar, Image,
  Link, Mail, AtSign, Palette, FileText, List,
  Braces, Code,
} from 'lucide-react';
import {
  DndContext, closestCenter, PointerSensor, useSensor, useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext, useSortable, verticalListSortingStrategy,
  arrayMove,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { pinyin } from 'pinyin-pro';
import { contentTypes } from '@/api/endpoints';
import { ApiError } from '@/api/client';
import { showError, showSuccess } from '@/components/Toast';
import { confirm } from '@/components/ConfirmDialog';
import type { Field, FieldType, ValidationRules } from '@/types';
import styles from './ContentTypeBuilder.module.css';

const { Title } = Typography;
const { TextArea } = Input;

const FIELD_TYPES: { value: FieldType; icon: React.ReactNode }[] = [
  { value: 'text', icon: <Type size={14} /> },
  { value: 'richtext', icon: <FileText size={14} /> },
  { value: 'number', icon: <Hash size={14} /> },
  { value: 'boolean', icon: <ToggleLeft size={14} /> },
  { value: 'date', icon: <Calendar size={14} /> },
  { value: 'datetime', icon: <Calendar size={14} /> },
  { value: 'media', icon: <Image size={14} /> },
  { value: 'relation', icon: <Link size={14} /> },
  { value: 'enum', icon: <List size={14} /> },
  { value: 'json', icon: <Braces size={14} /> },
  { value: 'email', icon: <Mail size={14} /> },
  { value: 'url', icon: <Link size={14} /> },
  { value: 'slug', icon: <AtSign size={14} /> },
  { value: 'color', icon: <Palette size={14} /> },
  { value: 'markdown', icon: <Code size={14} /> },
];

function slugify(text: string): string {
  const py = pinyin(text, { toneType: 'none', type: 'array' }).join('_');
  return py
    .toLowerCase()
    .replace(/[^a-z0-9_]/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_|_$/g, '');
}

function SortableFieldItem({
  field,
  index,
  isExpanded,
  onToggleExpand,
  onUpdate,
  onRemove,
  t,
}: {
  field: Field;
  index: number;
  isExpanded: boolean;
  onToggleExpand: () => void;
  onUpdate: (f: Field) => void;
  onRemove: () => void;
  t: (key: string) => string;
}) {
  const {
    attributes, listeners, setNodeRef, transform, transition, isDragging,
  } = useSortable({ id: field.name });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  const handleNameChange = (val: string) => {
    onUpdate({ ...field, display_name: val, name: slugify(val) });
  };

  const handleSlugChange = (val: string) => {
    onUpdate({ ...field, name: slugify(val) });
  };

  const handleTypeChange = (val: string) => {
    onUpdate({ ...field, type: val as FieldType });
  };

  const handleToggleRequired = (val: boolean) => {
    onUpdate({ ...field, required: val });
  };

  const handleToggleUnique = (val: boolean) => {
    onUpdate({ ...field, unique: val });
  };

  const handleDefaultChange = (val: string) => {
    onUpdate({ ...field, default_value: val || undefined });
  };

  const handleValidation = (key: keyof ValidationRules, value: number | string | string[] | undefined) => {
    const newValidation = { ...field.validation };
    if (value === undefined || value === '' || (Array.isArray(value) && value.length === 0)) {
      delete newValidation[key];
    } else {
      newValidation[key] = value as never;
    }
    onUpdate({ ...field, validation: Object.keys(newValidation).length > 0 ? newValidation : undefined });
  };

  const isTextType = ['text', 'richtext', 'markdown', 'email', 'url', 'slug'].includes(field.type);
  const isNumberType = field.type === 'number';
  const isEnumType = field.type === 'enum';

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`${styles.fieldCard} ${isExpanded ? styles.fieldCardExpanded : ''} ${isDragging ? styles.fieldCardDragging : ''}`}
    >
      <div className={styles.fieldHeader}>
        <div className={styles.dragHandle} {...attributes} {...listeners}>
          <GripVertical size={16} />
        </div>

        <div className={styles.fieldInfo}>
          <span className={styles.fieldDisplayName}>
            {field.display_name || `Field ${index + 1}`}
          </span>
          {field.name && (
            <span className={styles.fieldNameSlug}>{field.name}</span>
          )}
          <span className={styles.fieldTypeBadge}>{field.type}</span>
          {field.required && (
            <span className={`${styles.fieldTypeBadge} ${styles.badgeRequired}`}>
              {t('content_type.field_required')}
            </span>
          )}
          {field.unique && (
            <span className={`${styles.fieldTypeBadge} ${styles.badgeUnique}`}>
              {t('content_type.field_unique')}
            </span>
          )}
        </div>

        <div className={styles.fieldActions}>
          <button
            type="button"
            className={styles.fieldActionBtn}
            onClick={onToggleExpand}
            title={t('common.edit')}
          >
            <Pencil size={14} />
          </button>
          <button
            type="button"
            className={`${styles.fieldActionBtn} ${styles.danger}`}
            onClick={() => {
              confirm({
                title: t('content_type.remove_field'),
                message: t('content_type.remove_field_confirm'),
                danger: true,
                confirmText: t('common.delete'),
                cancelText: t('common.cancel'),
                onConfirm: onRemove,
              });
            }}
            title={t('common.delete')}
          >
            <Trash2 size={14} />
          </button>
        </div>
      </div>

      {isExpanded && (
        <div className={styles.fieldConfig}>
          <div className={styles.configGrid}>
            <div>
              <span className={styles.formLabel}>{t('content_type.field_display_name')}</span>
              <Input
                value={field.display_name}
                onChange={handleNameChange}
                placeholder="My Field"
              />
            </div>
            <div>
              <span className={styles.formLabel}>{t('content_type.field_name')}</span>
              <Input
                value={field.name}
                onChange={handleSlugChange}
                placeholder="my_field"
              />
            </div>
            <div className={styles.fullWidth}>
              <span className={styles.formLabel}>{t('content_type.field_type')}</span>
              <Select
                value={field.type}
                onChange={handleTypeChange}
                showSearch
                style={{ width: '100%' }}
              >
                {FIELD_TYPES.map((ft) => (
                  <Select.Option key={ft.value} value={ft.value}>
                    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                      {ft.icon} {ft.value}
                    </span>
                  </Select.Option>
                ))}
              </Select>
            </div>
            <div className={styles.fullWidth}>
              <span className={styles.formLabel}>{t('content_type.field_default')}</span>
              <Input
                value={typeof field.default_value === 'string' ? field.default_value : ''}
                onChange={handleDefaultChange}
                placeholder={t('content_type.field_default')}
              />
            </div>
          </div>

          <div className={styles.togglesRow}>
            <div className={styles.toggleItem}>
              <Switch
                size="small"
                checked={field.required}
                onChange={handleToggleRequired}
              />
              <span>{t('content_type.field_required')}</span>
            </div>
            <div className={styles.toggleItem}>
              <Switch
                size="small"
                checked={field.unique}
                onChange={handleToggleUnique}
              />
              <span>{t('content_type.field_unique')}</span>
            </div>
          </div>

          {(isTextType || isNumberType || isEnumType) && (
            <div className={styles.validationSection}>
              <div className={styles.validationTitle}>{t('content_type.validation')}</div>
              <div className={styles.validationGrid}>
                {isTextType && (
                  <>
                    <div>
                      <span className={styles.formLabel}>Min Length</span>
                      <InputNumber
                        min={0}
                        value={field.validation?.min_length}
                        onChange={(v) => handleValidation('min_length', v ?? undefined)}
                        placeholder="0"
                        style={{ width: '100%' }}
                      />
                    </div>
                    <div>
                      <span className={styles.formLabel}>Max Length</span>
                      <InputNumber
                        min={0}
                        value={field.validation?.max_length}
                        onChange={(v) => handleValidation('max_length', v ?? undefined)}
                        placeholder="∞"
                        style={{ width: '100%' }}
                      />
                    </div>
                    <div className={styles.fullWidth}>
                      <span className={styles.formLabel}>Pattern (regex)</span>
                      <Input
                        value={field.validation?.pattern ?? ''}
                        onChange={(v) => handleValidation('pattern', v || undefined)}
                        placeholder="^[a-z]+$"
                      />
                    </div>
                  </>
                )}
                {isNumberType && (
                  <>
                    <div>
                      <span className={styles.formLabel}>Min Value</span>
                      <InputNumber
                        value={field.validation?.min}
                        onChange={(v) => handleValidation('min', v ?? undefined)}
                        style={{ width: '100%' }}
                      />
                    </div>
                    <div>
                      <span className={styles.formLabel}>Max Value</span>
                      <InputNumber
                        value={field.validation?.max}
                        onChange={(v) => handleValidation('max', v ?? undefined)}
                        style={{ width: '100%' }}
                      />
                    </div>
                  </>
                )}
                {isEnumType && (
                  <div className={styles.fullWidth}>
                    <span className={styles.formLabel}>Values (comma-separated)</span>
                    <Input
                      value={(field.validation?.values ?? []).join(', ')}
                      onChange={(v) => {
                        const values = v.split(',').map((s) => s.trim()).filter(Boolean);
                        handleValidation('values', values.length > 0 ? values : undefined);
                      }}
                      placeholder="draft, published, archived"
                    />
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default function ContentTypeBuilder() {
  const { t } = useTranslation();
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const isEditing = Boolean(name);

  const [displayName, setDisplayName] = useState('');
  const [ctName, setCtName] = useState('');
  const [description, setDescription] = useState('');
  const [fields, setFields] = useState<Field[]>([]);
  const [expandedField, setExpandedField] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
  );

  const fetchCT = useCallback(async () => {
    if (!name) return;
    setLoading(true);
    try {
      const ct = await contentTypes.get(name);
      setDisplayName(ct.display_name);
      setCtName(ct.name);
      setDescription(ct.description);
      setFields(ct.fields ?? []);
    } catch (err) {
      if (err instanceof ApiError) {
        showError(err.message);
      } else {
        showError(t('common.error_occurred'));
      }
    } finally {
      setLoading(false);
    }
  }, [name, t]);

  useEffect(() => {
    fetchCT();
  }, [fetchCT]);

  const handleDisplayNameChange = (val: string) => {
    setDisplayName(val);
    if (!isEditing) {
      setCtName(slugify(val));
    }
  };

  const addField = (type: FieldType) => {
    const newField: Field = {
      name: `field_${fields.length + 1}`,
      display_name: '',
      type,
      required: false,
      unique: false,
    };
    setFields((prev) => [...prev, newField]);
    setExpandedField(newField.name);
  };

  const updateField = (index: number, updated: Field) => {
    setFields((prev) => prev.map((f, i) => (i === index ? updated : f)));
  };

  const removeField = (index: number) => {
    const removedName = fields[index].name;
    setFields((prev) => prev.filter((_, i) => i !== index));
    if (expandedField === removedName) {
      setExpandedField(null);
    }
  };

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (over && active.id !== over.id) {
      setFields((prev) => {
        const oldIndex = prev.findIndex((f) => f.name === active.id);
        const newIndex = prev.findIndex((f) => f.name === over.id);
        return arrayMove(prev, oldIndex, newIndex);
      });
    }
  };

  const handleSave = async () => {
    if (!displayName.trim()) {
      showError('Display name is required');
      return;
    }
    if (!ctName.trim()) {
      showError('Name is required');
      return;
    }
    for (const f of fields) {
      if (!f.name.trim()) {
        showError(`Field "${f.display_name || 'unnamed'}" has no API name`);
        return;
      }
    }

    setSaving(true);
    try {
      if (isEditing && name) {
        await contentTypes.update(name, {
          display_name: displayName.trim(),
          description: description.trim(),
          fields: fields as any,
        });
      } else {
        await contentTypes.create({
          name: ctName.trim(),
          display_name: displayName.trim(),
          description: description.trim(),
          fields: fields as any,
        });
      }
      showSuccess(t('content_type.saved_success'));
      navigate('/admin/content-types');
    } catch (err) {
      if (err instanceof ApiError) {
        showError(err.message);
      } else {
        showError(t('common.error_occurred'));
      }
    } finally {
      setSaving(false);
    }
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

  const addFieldDropdownItems = FIELD_TYPES.map((ft) => ({
    key: ft.value,
    label: (
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
        {ft.icon} {ft.value}
      </span>
    ),
  }));

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <button
          type="button"
          className={styles.backBtn}
          onClick={() => navigate('/admin/content-types')}
        >
          <ArrowLeft size={18} />
        </button>
        <div className={styles.headerTitle}>
          <Title heading={5} style={{ margin: 0 }}>
            {isEditing ? t('content_type.edit') : t('content_type.create')}
          </Title>
        </div>
      </div>

      <div className={styles.body}>
        <Card className={styles.infoCard}>
          <div className={styles.cardTitle}>
            {t('common.name')}
          </div>
          <div className={styles.formGroup}>
            <span className={styles.formLabel}>
              {t('content_type.display_name_label')}
            </span>
            <Input
              value={displayName}
              onChange={handleDisplayNameChange}
              placeholder="e.g. Blog Post"
            />
          </div>
          <div className={styles.formGroup}>
            <span className={styles.formLabel}>{t('content_type.field_name')}</span>
            <Input
              value={ctName}
              onChange={(v) => setCtName(slugify(v))}
              disabled={isEditing}
              placeholder="blog_post"
            />
            <div className={styles.formHint}>{t('content_type.name_help')}</div>
          </div>
          <div className={styles.formGroup}>
            <span className={styles.formLabel}>{t('common.description')}</span>
            <TextArea
              value={description}
              onChange={setDescription}
              placeholder={t('common.description')}
              autoSize={{ minRows: 2, maxRows: 4 }}
            />
          </div>
        </Card>

        <Card className={styles.fieldsCard}>
          <div className={styles.cardTitle}>
            {t('content_type.fields_section')}
            <Tag size="small">{fields.length}</Tag>
          </div>

          <div className={styles.addFieldBar}>
            <Dropdown
              trigger="click"
              droplist={
                <Menu
                  onClickMenuItem={(key) => addField(key as FieldType)}
                >
                  {addFieldDropdownItems.map((item) => (
                    <Menu.Item key={item.key}>{item.label}</Menu.Item>
                  ))}
                </Menu>
              }
            >
              <Button type="primary" icon={<Plus size={16} />}>
                {t('content_type.add_field')}
              </Button>
            </Dropdown>
          </div>

          {fields.length === 0 ? (
            <div className={styles.emptyFields}>
              <div className={styles.emptyFieldsIcon}>
                <Type size={28} />
              </div>
              <div className={styles.emptyFieldsText}>
                {t('content_type.no_types')}
              </div>
              <div className={styles.emptyFieldsHint}>
                {t('content_type.drag_to_reorder')}
              </div>
            </div>
          ) : (
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragEnd={handleDragEnd}
            >
              <SortableContext
                items={fields.map((f) => f.name)}
                strategy={verticalListSortingStrategy}
              >
                <div className={styles.fieldList}>
                  {fields.map((field, idx) => (
                    <SortableFieldItem
                      key={field.name}
                      field={field}
                      index={idx}
                      isExpanded={expandedField === field.name}
                      onToggleExpand={() =>
                        setExpandedField(expandedField === field.name ? null : field.name)
                      }
                      onUpdate={(f) => updateField(idx, f)}
                      onRemove={() => removeField(idx)}
                      t={t}
                    />
                  ))}
                </div>
              </SortableContext>
            </DndContext>
          )}
        </Card>
      </div>

      <div className={styles.footer}>
        <Button onClick={() => navigate('/admin/content-types')}>
          {t('common.cancel')}
        </Button>
        <Button
          type="primary"
          icon={<Save size={16} />}
          loading={saving}
          onClick={handleSave}
        >
          {t('common.save')}
        </Button>
      </div>
    </div>
  );
}

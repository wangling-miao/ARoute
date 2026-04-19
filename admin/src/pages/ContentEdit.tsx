import { useEffect, useState, useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams, useNavigate, Link } from 'react-router-dom';
import {
  Card,
  Button,
  Input,
  InputNumber,
  Select,
  Switch,
  DatePicker,
  Tag,
} from '@arco-design/web-react';
import {
  ChevronRight,
  Save,
  ArrowLeft,
  AlertCircle,
} from 'lucide-react';
import { content, contentTypes } from '@/api/endpoints';
import type { ContentType, ContentItem, Field } from '@/types';
import { showSuccess, showError } from '@/components/Toast';
import RichTextEditor from '@/components/RichTextEditor';
import styles from './ContentEdit.module.css';

const { Option } = Select;
const { TextArea } = Input;

function slugify(text: string): string {
  return text
    .toLowerCase()
    .trim()
    .replace(/[^\w\s-]/g, '')
    .replace(/[\s_]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-+|-+$/g, '');
}

interface FieldRenderProps {
  field: Field;
  value: unknown;
  onChange: (val: unknown) => void;
  errors: Record<string, string>;
  ctDef: ContentType;
  formData: Record<string, unknown>;
}

function FieldRenderer({ field, value, onChange, errors, ctDef, formData }: FieldRenderProps) {
  const { t } = useTranslation();

  const strVal = typeof value === 'string' ? value : '';
  const numVal = typeof value === 'number' ? value : undefined;
  const boolVal = typeof value === 'boolean' ? value : false;

  const label = (
    <span className={`${styles.formLabel} ${field.required ? styles.formRequired : ''}`}>
      {field.display_name}
    </span>
  );

  const errorEl = errors[field.name] ? (
    <div className={styles.formError}>{errors[field.name]}</div>
  ) : null;

  switch (field.type) {
    case 'text':
    case 'email':
    case 'url':
      return (
        <div className={styles.formRow}>
          {label}
          <Input
            value={strVal}
            onChange={(v: string) => onChange(v)}
            placeholder={field.display_name}
            type={field.type === 'email' ? 'email' : field.type === 'url' ? 'url' : 'text'}
            maxLength={field.validation?.max_length}
          />
          {errorEl}
        </div>
      );

    case 'slug': {
      const titleField = ctDef.fields.find(f => f.type === 'text')?.name;
      return (
        <div className={styles.formRow}>
          {label}
          <div className={styles.slugRow}>
            <Input
              className={styles.slugInput}
              value={strVal}
              onChange={(v: string) => onChange(v)}
              placeholder={field.display_name}
            />
            {titleField && (
              <Button
                className={styles.slugGenerateBtn}
                onClick={() => {
                  const titleVal = formData[titleField];
                  onChange(slugify(typeof titleVal === 'string' ? titleVal : ''));
                }}
                size="small"
              >
                {t('content.generate_slug')}
              </Button>
            )}
          </div>
          {errorEl}
        </div>
      );
    }

    case 'richtext':
      return (
        <div className={styles.formRow}>
          {label}
          <RichTextEditor
            value={strVal}
            onChange={(html: string) => onChange(html)}
            placeholder={field.display_name}
          />
          {errorEl}
        </div>
      );

    case 'markdown':
      return (
        <div className={styles.formRow}>
          {label}
          <TextArea
            value={strVal}
            onChange={(v: string) => onChange(v)}
            placeholder={field.display_name}
            autoSize={{ minRows: 6, maxRows: 20 }}
          />
          {errorEl}
        </div>
      );

    case 'number':
      return (
        <div className={styles.formRow}>
          {label}
          <InputNumber
            value={numVal}
            onChange={(v: number | undefined) => onChange(v)}
            placeholder={field.display_name}
            min={field.validation?.min}
            max={field.validation?.max}
            style={{ width: '100%' }}
          />
          {errorEl}
        </div>
      );

    case 'boolean':
      return (
        <div className={styles.formRow}>
          {label}
          <Switch checked={boolVal} onChange={(v: boolean) => onChange(v)} />
          {errorEl}
        </div>
      );

    case 'date':
      return (
        <div className={styles.formRow}>
          {label}
          <DatePicker
            value={strVal ? strVal : undefined}
            onChange={(_dateString: string, _date: any) => onChange(_dateString)}
            style={{ width: '100%' }}
          />
          {errorEl}
        </div>
      );

    case 'datetime':
      return (
        <div className={styles.formRow}>
          {label}
          <DatePicker
            showTime
            value={strVal ? strVal : undefined}
            onChange={(_dateString: string, _date: any) => onChange(_dateString)}
            style={{ width: '100%' }}
          />
          {errorEl}
        </div>
      );

    case 'media':
      return (
        <div className={styles.formRow}>
          {label}
          <Input
            value={strVal}
            onChange={(v: string) => onChange(v)}
            placeholder={t('content.media_url_placeholder')}
          />
          {errorEl}
        </div>
      );

    case 'relation':
      return (
        <div className={styles.formRow}>
          {label}
          <Input
            value={strVal}
            onChange={(v: string) => onChange(v)}
            placeholder={t('content.relation_placeholder')}
          />
          {errorEl}
        </div>
      );

    case 'enum': {
      const enumValues = field.validation?.values || [];
      return (
        <div className={styles.formRow}>
          {label}
          <Select
            value={strVal || undefined}
            onChange={(v: string) => onChange(v)}
            placeholder={t('common.select_all') + '...'}
            allowClear
          >
            {enumValues.map((v: string) => (
              <Option key={v} value={v}>{v}</Option>
            ))}
          </Select>
          {errorEl}
        </div>
      );
    }

    case 'json':
      return (
        <div className={styles.formRow}>
          {label}
          <TextArea
            value={strVal}
            onChange={(v: string) => onChange(v)}
            placeholder="{}"
            autoSize={{ minRows: 4, maxRows: 12 }}
          />
          {errorEl}
        </div>
      );

    case 'color':
      return (
        <div className={styles.formRow}>
          {label}
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <input
              type="color"
              value={strVal || '#000000'}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
              style={{ width: 40, height: 36, border: '1px solid var(--color-border)', borderRadius: 'var(--radius-sm)', cursor: 'pointer', padding: 2 }}
            />
            <Input
              value={strVal}
              onChange={(v: string) => onChange(v)}
              placeholder="#000000"
              style={{ width: 140 }}
            />
          </div>
          {errorEl}
        </div>
      );

    default:
      return (
        <div className={styles.formRow}>
          {label}
          <Input
            value={strVal}
            onChange={(v: string) => onChange(v)}
            placeholder={field.display_name}
          />
        </div>
      );
  }
}

export default function ContentEdit() {
  const { t } = useTranslation();
  const { contentType, id } = useParams<{ contentType: string; id: string }>();
  const navigate = useNavigate();
  const isNew = !id;

  const [ctDef, setCtDef] = useState<ContentType | null>(null);
  const [formData, setFormData] = useState<Record<string, unknown>>({});
  const [status, setStatus] = useState<'draft' | 'published'>('draft');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const fetchData = useCallback(async () => {
    if (!contentType) return;
    try {
      setLoading(true);
      const ct = await contentTypes.get(contentType);
      setCtDef(ct);

      const defaults: Record<string, unknown> = {};
      ct.fields.forEach((f) => {
        if (f.default_value !== undefined && f.default_value !== null) {
          defaults[f.name] = f.default_value;
        } else if (f.type === 'boolean') {
          defaults[f.name] = false;
        } else if (f.type === 'number') {
          defaults[f.name] = 0;
        } else {
          defaults[f.name] = '';
        }
      });

      if (id) {
        const item: ContentItem = await content.get(contentType, id);
        const data: Record<string, unknown> = { ...defaults, ...item.data };
        setFormData(data);
        setStatus(item.status);
      } else {
        setFormData(defaults);
      }
    } catch {
      showError(t('common.error_occurred'));
    } finally {
      setLoading(false);
    }
  }, [contentType, id, t]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const handleFieldChange = useCallback((fieldName: string, value: unknown) => {
    setFormData(prev => ({ ...prev, [fieldName]: value }));
    setErrors(prev => {
      const next = { ...prev };
      delete next[fieldName];
      return next;
    });
  }, []);

  const validate = useCallback((): boolean => {
    if (!ctDef) return false;
    const newErrors: Record<string, string> = {};

    ctDef.fields.forEach((field) => {
      const val = formData[field.name];

      if (field.required) {
        if (val === undefined || val === null || val === '' || val === false) {
          newErrors[field.name] = t('content.field_required', { field: field.display_name });
        }
      }

      if (field.type === 'email' && typeof val === 'string' && val.trim()) {
        if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(val)) {
          newErrors[field.name] = t('content.field_invalid_email');
        }
      }

      if (field.type === 'url' && typeof val === 'string' && val.trim()) {
        try {
          new URL(val);
        } catch {
          newErrors[field.name] = t('content.field_invalid_url');
        }
      }

      if (field.type === 'json' && typeof val === 'string' && val.trim()) {
        try {
          JSON.parse(val);
        } catch {
          newErrors[field.name] = t('content.field_invalid_json');
        }
      }

      if (field.validation?.min_length && typeof val === 'string' && val.length < field.validation.min_length) {
        newErrors[field.name] = t('content.field_min_length', { min: field.validation.min_length });
      }

      if (field.validation?.max_length && typeof val === 'string' && val.length > field.validation.max_length) {
        newErrors[field.name] = t('content.field_max_length', { max: field.validation.max_length });
      }
    });

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  }, [ctDef, formData, t]);

  const handleSave = useCallback(async () => {
    if (!contentType || !ctDef) return;
    if (!validate()) return;

    try {
      setSaving(true);
      const payload = { ...formData, status };
      if (id) {
        await content.update(contentType, id, payload);
      } else {
        await content.create(contentType, payload);
      }
      showSuccess(t('content.saved_success'));
      navigate(`/admin/content/${contentType}`);
    } catch {
      showError(t('common.error_occurred'));
    } finally {
      setSaving(false);
    }
  }, [contentType, ctDef, id, formData, status, validate, navigate, t]);

  const pageTitle = useMemo(() => {
    if (!ctDef) return '';
    return isNew
      ? t('content.create', { type: ctDef.display_name })
      : t('content.edit', { type: ctDef.display_name });
  }, [ctDef, isNew, t]);

  if (loading) {
    return (
      <div className={styles.page}>
        <div className={`skeleton ${styles.skeletonForm}`} />
      </div>
    );
  }

  if (!ctDef || !contentType) {
    return (
      <div className={styles.page}>
        <div className={styles.errorState}>
          <AlertCircle size={40} style={{ marginBottom: 12, opacity: 0.4 }} />
          <p>{t('common.error_occurred')}</p>
          <Button onClick={() => navigate(-1)} style={{ marginTop: 12 }}>
            {t('common.go_back')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.page}>
      <div className={styles.breadcrumb}>
        <Link to="/admin/content" className={styles.breadcrumbLink}>
          {t('nav.content')}
        </Link>
        <ChevronRight size={14} className={styles.breadcrumbSep} />
        <Link to={`/admin/content/${contentType}`} className={styles.breadcrumbLink}>
          {ctDef.display_name}
        </Link>
        <ChevronRight size={14} className={styles.breadcrumbSep} />
        <span className={styles.breadcrumbCurrent}>
          {isNew ? t('common.create') : t('common.edit')}
        </span>
      </div>

      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <Button
            type="text"
            icon={<ArrowLeft size={18} />}
            onClick={() => navigate(`/admin/content/${contentType}`)}
          />
          <h2 className={styles.headerTitle}>{pageTitle}</h2>
        </div>
        <div className={styles.headerActions}>
          <div className={styles.statusToggle}>
            <span>{t('common.status')}:</span>
            <Switch
              checked={status === 'published'}
              onChange={(checked: boolean) => setStatus(checked ? 'published' : 'draft')}
            />
            <Tag color={status === 'published' ? 'green' : 'orange'} size="small">
              {status === 'published' ? t('content.status_published') : t('content.status_draft')}
            </Tag>
          </div>
          <Button type="primary" icon={<Save size={16} />} loading={saving} onClick={handleSave}>
            {t('common.save')}
          </Button>
        </div>
      </div>

      <Card className={styles.formCard} bordered={false}>
        {ctDef.fields.map((field) => (
          <FieldRenderer
            key={field.name}
            field={field}
            value={formData[field.name]}
            onChange={(val: unknown) => handleFieldChange(field.name, val)}
            errors={errors}
            ctDef={ctDef}
            formData={formData}
          />
        ))}
      </Card>
    </div>
  );
}

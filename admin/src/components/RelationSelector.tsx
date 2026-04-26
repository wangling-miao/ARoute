import { useEffect, useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Select, Spin } from '@arco-design/web-react';
import { content } from '@/api/endpoints';
import type { ContentItem, RelationConfig } from '@/types';

interface Props {
  relationConfig: RelationConfig;
  value: unknown;
  onChange: (val: unknown) => void;
  placeholder?: string;
}

interface Option {
  label: string;
  value: string;
}

export default function RelationSelector({ relationConfig, value, onChange, placeholder }: Props) {
  const { t } = useTranslation();
  const [options, setOptions] = useState<Option[]>([]);
  const [loading, setLoading] = useState(true);

  const isMultiple = relationConfig.relation_type === 'many-to-many';

  useEffect(() => {
    let cancelled = false;
    content.list(relationConfig.target_content_type, { per_page: 200 })
      .then(res => {
        if (cancelled) return;
        const items = res.data ?? [];
        setOptions(items.map((item: ContentItem) => ({
          label: (item.name as string) || (item.title as string) || item.id.slice(0, 8),
          value: item.id,
        })));
      })
      .catch(() => {})
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [relationConfig.target_content_type]);

  const selectedValue = useMemo(() => {
    if (isMultiple) {
      if (Array.isArray(value)) return value.map(String);
      if (value == null) return [];
      return [String(value)];
    }
    if (value == null || value === '') return undefined;
    return String(value);
  }, [value, isMultiple]);

  if (loading) {
    return <Spin size={16} />;
  }

  return (
    <Select
      mode={isMultiple ? 'multiple' : undefined}
      showSearch
      filterOption={(input, option) => {
        const label = (option?.props as Record<string, unknown>)?.children as string | undefined;
        return label?.toLowerCase().includes(input.toLowerCase()) ?? false;
      }}
      value={selectedValue as string & string[]}
      onChange={(val: string | string[]) => onChange(val)}
      placeholder={placeholder || t('content.relation_placeholder')}
      allowClear
      style={{ width: '100%' }}
    >
      {options.map(opt => (
        <Select.Option key={opt.value} value={opt.value}>
          {opt.label}
        </Select.Option>
      ))}
    </Select>
  );
}

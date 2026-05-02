import { useState, useEffect, useCallback, useRef } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { Input, Button, Spin } from '@arco-design/web-react';
import { Upload, Search, Image, X, Film, Music, FileText, File } from 'lucide-react';
import { media } from '@/api/endpoints';
import { ApiError } from '@/api/client';
import { showError, showSuccess } from '@/components/Toast';
import type { MediaFile } from '@/types';
import styles from './MediaPicker.module.css';

type FileCategory = 'all' | 'image' | 'video' | 'audio' | 'document' | 'other';

interface UploadProgress {
  file: File;
  progress: number;
  status: 'uploading' | 'done' | 'error';
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function getFileCategory(mime: string): FileCategory {
  if (mime.startsWith('image/')) return 'image';
  if (mime.startsWith('video/')) return 'video';
  if (mime.startsWith('audio/')) return 'audio';
  if (
    mime.startsWith('text/') ||
    mime === 'application/pdf' ||
    mime === 'application/json' ||
    mime.includes('spreadsheet') ||
    mime.includes('document') ||
    mime.includes('presentation') ||
    mime.includes('csv') ||
    mime.includes('xml') ||
    mime.includes('zip') ||
    mime.includes('compressed')
  ) return 'document';
  return 'other';
}

function getCategoryIcon(category: FileCategory, size = 16) {
  switch (category) {
    case 'image': return <Image size={size} />;
    case 'video': return <Film size={size} />;
    case 'audio': return <Music size={size} />;
    case 'document': return <FileText size={size} />;
    default: return <File size={size} />;
  }
}

const categories: { key: FileCategory; labelKey: string }[] = [
  { key: 'all', labelKey: 'common.all' },
  { key: 'image', labelKey: 'media.images' },
  { key: 'document', labelKey: 'media.documents' },
  { key: 'video', labelKey: 'media.videos' },
  { key: 'audio', labelKey: 'media.audio' },
  { key: 'other', labelKey: 'media.other' },
];

interface MediaPickerProps {
  open: boolean;
  onSelect: (file: MediaFile) => void;
  onClose: () => void;
  filter?: 'image' | 'all';
}

export default function MediaPicker({ open, onSelect, onClose, filter = 'all' }: MediaPickerProps) {
  const { t } = useTranslation();
  const [files, setFiles] = useState<MediaFile[]>([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState<FileCategory>(filter === 'image' ? 'image' : 'all');
  const [uploads, setUploads] = useState<UploadProgress[]>([]);
  const [dragActive, setDragActive] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const perPage = 20;

  const fetchFiles = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, unknown> = { page, per_page: perPage };
      if (search.trim()) params.search = search.trim();
      const res = await media.list(params);
      setFiles(res.data);
      setTotal(res.meta.total);
    } catch (err) {
      if (err instanceof ApiError) showError(err.message);
    } finally {
      setLoading(false);
    }
  }, [page, search]);

  useEffect(() => {
    if (open) fetchFiles();
  }, [open, fetchFiles]);

  // Reset state when opening
  useEffect(() => {
    if (open) {
      setPage(1);
      setSearch('');
      setCategory(filter === 'image' ? 'image' : 'all');
    }
  }, [open, filter]);

  const filteredFiles = category === 'all'
    ? files
    : files.filter((f) => getFileCategory(f.mime_type) === category);

  const handleUpload = async (fileList: FileList | File[]) => {
    const newUploads: UploadProgress[] = Array.from(fileList).map((file) => ({
      file, progress: 0, status: 'uploading' as const,
    }));
    setUploads((prev) => [...prev, ...newUploads]);

    let successCount = 0;
    for (const uploadItem of newUploads) {
      try {
        await media.upload(uploadItem.file, (pct) => {
          setUploads((prev) =>
            prev.map((u) => u.file === uploadItem.file ? { ...u, progress: pct } : u),
          );
        });
        successCount++;
        setUploads((prev) =>
          prev.map((u) => u.file === uploadItem.file ? { ...u, status: 'done' as const } : u),
        );
      } catch {
        setUploads((prev) =>
          prev.map((u) => u.file === uploadItem.file ? { ...u, status: 'error' as const } : u),
        );
      }
    }

    if (successCount > 0) {
      showSuccess(t('media.uploaded_success', { count: successCount }));
      fetchFiles();
    }

    setTimeout(() => {
      setUploads((prev) => prev.filter((u) => u.status === 'uploading'));
    }, 3000);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragActive(false);
    if (e.dataTransfer.files.length > 0) handleUpload(e.dataTransfer.files);
  };

  const totalPages = Math.ceil(total / perPage);

  // Keyboard: Escape to close
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [open, onClose]);

  if (!open) return null;

  return createPortal(
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.header}>
          <h3 className={styles.title}>{t('media.title')}</h3>
          <div className={styles.headerActions}>
            <Button
              type="primary"
              size="small"
              icon={<Upload size={14} />}
              onClick={() => fileInputRef.current?.click()}
            >
              {t('media.upload')}
            </Button>
            <button type="button" className={styles.closeBtn} onClick={onClose}>
              <X size={18} />
            </button>
          </div>
        </div>

        <div className={styles.toolbar}>
          <div className={styles.categoryTabs}>
            {categories
              .filter(c => filter === 'all' || c.key === 'all' || c.key === filter)
              .map((c) => (
                <button
                  key={c.key}
                  type="button"
                  className={`${styles.categoryTab} ${category === c.key ? styles.categoryTabActive : ''}`}
                  onClick={() => setCategory(c.key)}
                >
                  {c.key !== 'all' && getCategoryIcon(c.key)}
                  {t(c.labelKey)}
                </button>
              ))}
          </div>
          <div className={styles.searchBar}>
            <Input
              prefix={<Search size={14} />}
              placeholder={t('common.search')}
              value={search}
              onChange={setSearch}
              allowClear
              size="small"
              onClear={() => { setSearch(''); setPage(1); }}
              onKeyDown={(e) => { if (e.key === 'Enter') { setPage(1); fetchFiles(); } }}
            />
          </div>
        </div>

        <input
          ref={fileInputRef}
          type="file"
          multiple
          style={{ display: 'none' }}
          onChange={(e) => {
            if (e.target.files && e.target.files.length > 0) {
              handleUpload(e.target.files);
              e.target.value = '';
            }
          }}
        />

        {uploads.length > 0 && (
          <div className={styles.uploadProgress}>
            {uploads.map((u) => (
              <div key={u.file.name} className={styles.uploadItem}>
                <Upload size={12} />
                <span className={styles.uploadFileName}>{u.file.name}</span>
                <span className={styles.uploadPercent}>{u.progress}%</span>
              </div>
            ))}
          </div>
        )}

        <div
          className={`${styles.dropzone} ${dragActive ? styles.dropzoneActive : ''}`}
          onDragOver={(e) => { e.preventDefault(); setDragActive(true); }}
          onDragLeave={() => setDragActive(false)}
          onDrop={handleDrop}
        >
          <Upload size={20} />
          <span>{t('media.upload_dropzone')}</span>
        </div>

        <div className={styles.gridWrapper}>
          {loading ? (
            <div className={styles.loadingWrap}><Spin size={32} /></div>
          ) : filteredFiles.length === 0 ? (
            <div className={styles.emptyState}>
              <Image size={24} />
              <span>{t('media.no_files')}</span>
            </div>
          ) : (
            <div className={styles.gridView}>
              {filteredFiles.map((mf) => {
                const cat = getFileCategory(mf.mime_type);
                return (
                  <button
                    key={mf.id}
                    type="button"
                    className={styles.mediaCard}
                    onClick={() => onSelect(mf)}
                  >
                    {cat === 'image' ? (
                      <img src={mf.url} alt={mf.filename} className={styles.mediaThumb} />
                    ) : (
                      <div className={styles.mediaThumbPlaceholder}>
                        {getCategoryIcon(cat, 24)}
                        <span className={styles.mediaThumbExt}>
                          {mf.filename.split('.').pop()?.toUpperCase() || 'FILE'}
                        </span>
                      </div>
                    )}
                    <div className={styles.mediaInfo}>
                      <div className={styles.mediaFilename}>{mf.filename}</div>
                      <div className={styles.mediaSize}>{formatFileSize(mf.size)}</div>
                    </div>
                  </button>
                );
              })}
            </div>
          )}
        </div>

        {totalPages > 1 && (
          <div className={styles.pagination}>
            <Button disabled={page <= 1} onClick={() => setPage(p => p - 1)} size="small">
              {t('common.previous')}
            </Button>
            <span className={styles.pageInfo}>{page} / {totalPages}</span>
            <Button disabled={page >= totalPages} onClick={() => setPage(p => p + 1)} size="small">
              {t('common.next')}
            </Button>
          </div>
        )}
      </div>
    </div>,
    document.body,
  );
}

import { useState, useEffect, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Input, Button, Spin, Table, Card, Typography } from '@arco-design/web-react';
import { Upload, Grid3X3, List, Search, Image, Trash2, File } from 'lucide-react';
import { media } from '@/api/endpoints';
import { ApiError } from '@/api/client';
import { showError, showSuccess } from '@/components/Toast';
import { confirm } from '@/components/ConfirmDialog';
import type { MediaFile } from '@/types';
import styles from './MediaLibrary.module.css';

const { Title } = Typography;

type ViewMode = 'grid' | 'list';

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

function isImageMime(mime: string): boolean {
  return mime.startsWith('image/');
}

export default function MediaLibrary() {
  const { t } = useTranslation();
  const [files, setFiles] = useState<MediaFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [search, setSearch] = useState('');
  const [viewMode, setViewMode] = useState<ViewMode>(
    () => (localStorage.getItem('media-view-mode') as ViewMode) || 'grid',
  );
  const [uploads, setUploads] = useState<UploadProgress[]>([]);
  const [dragActive, setDragActive] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const perPage = 20;

  const persistViewMode = (mode: ViewMode) => {
    setViewMode(mode);
    localStorage.setItem('media-view-mode', mode);
  };

  const fetchFiles = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const params: Record<string, unknown> = { page, per_page: perPage };
      if (search.trim()) {
        params.search = search.trim();
      }
      const res = await media.list(params);
      setFiles(res.data);
      setTotal(res.meta.total);
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
  }, [page, search, t]);

  useEffect(() => {
    fetchFiles();
  }, [fetchFiles]);

  const handleUpload = async (fileList: FileList | File[]) => {
    const newUploads: UploadProgress[] = Array.from(fileList).map((file) => ({
      file,
      progress: 0,
      status: 'uploading' as const,
    }));
    setUploads((prev) => [...prev, ...newUploads]);

    let successCount = 0;
    for (let i = 0; i < newUploads.length; i++) {
      const uploadItem = newUploads[i];
      try {
        await media.upload(uploadItem.file, (pct) => {
          setUploads((prev) =>
            prev.map((u) =>
              u.file === uploadItem.file ? { ...u, progress: pct } : u,
            ),
          );
        });
        successCount++;
        setUploads((prev) =>
          prev.map((u) =>
            u.file === uploadItem.file ? { ...u, status: 'done' as const } : u,
          ),
        );
      } catch {
        setUploads((prev) =>
          prev.map((u) =>
            u.file === uploadItem.file ? { ...u, status: 'error' as const } : u,
          ),
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
    if (e.dataTransfer.files.length > 0) {
      handleUpload(e.dataTransfer.files);
    }
  };

  const handleDelete = (mf: MediaFile) => {
    confirm({
      title: t('common.delete'),
      message: t('media.delete_confirm'),
      danger: true,
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      onConfirm: async () => {
        try {
          await media.delete(mf.id);
          showSuccess(t('media.deleted_success'));
          setFiles((prev) => prev.filter((f) => f.id !== mf.id));
          setTotal((prev) => prev - 1);
        } catch (err) {
          if (err instanceof ApiError) {
            showError(err.message);
          }
        }
      },
    });
  };

  const totalPages = Math.ceil(total / perPage);

  const columns = [
    {
      title: '',
      key: 'thumb',
      width: 70,
      render: (_: unknown, record: MediaFile) =>
        isImageMime(record.mime_type) ? (
          <img src={record.url} alt="" className={styles.thumbCell} />
        ) : (
          <div className={styles.thumbCellPlaceholder}>
            <File size={20} />
          </div>
        ),
    },
    { title: t('media.filename'), dataIndex: 'filename', key: 'filename' },
    { title: t('media.mime_type'), dataIndex: 'mime_type', key: 'mime_type', width: 140 },
    {
      title: t('media.size'),
      dataIndex: 'size',
      key: 'size',
      width: 100,
      render: (v: number) => formatFileSize(v),
    },
    {
      title: t('media.dimensions'),
      key: 'dims',
      width: 120,
      render: (_: unknown, record: MediaFile) =>
        record.width && record.height ? `${record.width}×${record.height}` : '—',
    },
    {
      title: t('common.actions'),
      key: 'actions',
      width: 60,
      render: (_: unknown, record: MediaFile) => (
        <button
          type="button"
          className={styles.deleteBtn}
          onClick={() => handleDelete(record)}
        >
          <Trash2 size={14} />
        </button>
      ),
    },
  ];

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <Title heading={5} style={{ margin: 0 }}>{t('media.title')}</Title>
        </div>
        <div className={styles.headerRight}>
          <Button
            type="primary"
            icon={<Upload size={16} />}
            onClick={() => fileInputRef.current?.click()}
          >
            {t('media.upload')}
          </Button>
          <div className={styles.viewToggle}>
            <button
              type="button"
              className={`${styles.viewBtn} ${viewMode === 'grid' ? styles.viewBtnActive : ''}`}
              onClick={() => persistViewMode('grid')}
            >
              <Grid3X3 size={16} />
            </button>
            <button
              type="button"
              className={`${styles.viewBtn} ${viewMode === 'list' ? styles.viewBtnActive : ''}`}
              onClick={() => persistViewMode('list')}
            >
              <List size={16} />
            </button>
          </div>
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

      <button
        type="button"
        className={`${styles.dropzone} ${dragActive ? styles.dropzoneActive : ''}`}
        onDragOver={(e) => { e.preventDefault(); setDragActive(true); }}
        onDragLeave={() => setDragActive(false)}
        onDrop={handleDrop}
        onClick={() => fileInputRef.current?.click()}
      >
        <div className={styles.dropzoneIcon}><Upload size={32} /></div>
        <div className={styles.dropzoneText}>{t('media.upload_dropzone')}</div>
        <div className={styles.dropzoneHint}>PNG, JPG, GIF, SVG, PDF, etc.</div>
      </button>

      {uploads.length > 0 && (
        <div className={styles.uploadProgress}>
          {uploads.map((u) => (
            <div key={u.file.name} className={styles.uploadItem}>
              <Upload size={14} />
              <span className={styles.uploadFileName}>{u.file.name}</span>
              <span className={styles.uploadPercent}>{u.progress}%</span>
            </div>
          ))}
        </div>
      )}

      <div className={styles.searchBar}>
        <Input
          prefix={<Search size={16} />}
          placeholder={t('common.search')}
          value={search}
          onChange={setSearch}
          allowClear
          onClear={() => { setSearch(''); setPage(1); }}
          onKeyDown={(e) => { if (e.key === 'Enter') { setPage(1); fetchFiles(); } }}
        />
      </div>

      {loading ? (
        <div className={styles.loadingWrap}><Spin size={40} /></div>
      ) : error ? (
        <div className={styles.emptyState}>
          <p>{t('common.error_occurred')}</p>
          <Button onClick={fetchFiles}>{t('common.try_again')}</Button>
        </div>
      ) : files.length === 0 ? (
        <div className={styles.emptyState}>
          <div className={styles.emptyIcon}><Image size={32} /></div>
          <div className={styles.emptyTitle}>{t('media.no_files')}</div>
        </div>
      ) : viewMode === 'grid' ? (
        <>
          <div className={styles.gridView}>
            {files.map((mf) => (
              <div key={mf.id} className={styles.mediaCard}>
                <div className={styles.mediaCardOverlay}>
                  <button
                    type="button"
                    className={styles.deleteBtn}
                    onClick={(e) => { e.stopPropagation(); handleDelete(mf); }}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
                {isImageMime(mf.mime_type) ? (
                  <img src={mf.url} alt={mf.filename} className={styles.mediaThumb} />
                ) : (
                  <div className={styles.mediaThumbPlaceholder}>
                    <File size={32} />
                  </div>
                )}
                <div className={styles.mediaInfo}>
                  <div className={styles.mediaFilename}>{mf.filename}</div>
                  <div className={styles.mediaSize}>{formatFileSize(mf.size)}</div>
                </div>
              </div>
            ))}
          </div>
          {totalPages > 1 && (
            <div className={styles.paginationWrap}>
              <Button
                disabled={page <= 1}
                onClick={() => setPage((p) => p - 1)}
                size="small"
              >
                {t('common.previous')}
              </Button>
              <span style={{ margin: '0 0.75rem', fontSize: '0.8125rem', color: 'var(--color-text-tertiary)' }}>
                {page} / {totalPages}
              </span>
              <Button
                disabled={page >= totalPages}
                onClick={() => setPage((p) => p + 1)}
                size="small"
              >
                {t('common.next')}
              </Button>
            </div>
          )}
        </>
      ) : (
        <>
          <Card className={styles.tableWrap}>
            <Table
              columns={columns}
              data={files}
              rowKey="id"
              pagination={false}
              border={false}
              borderCell={false}
            />
          </Card>
          {totalPages > 1 && (
            <div className={styles.paginationWrap}>
              <Button
                disabled={page <= 1}
                onClick={() => setPage((p) => p - 1)}
                size="small"
              >
                {t('common.previous')}
              </Button>
              <span style={{ margin: '0 0.75rem', fontSize: '0.8125rem', color: 'var(--color-text-tertiary)' }}>
                {page} / {totalPages}
              </span>
              <Button
                disabled={page >= totalPages}
                onClick={() => setPage((p) => p + 1)}
                size="small"
              >
                {t('common.next')}
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  );
}

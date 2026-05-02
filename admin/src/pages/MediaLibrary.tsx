import { useState, useEffect, useCallback, useRef } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { Input, Button, Spin, Table, Card, Typography } from '@arco-design/web-react';
import { Upload, Grid3X3, List, Search, Image, Trash2, File, X, Film, Music, FileText, Copy, Check, Download } from 'lucide-react';
import { common, createLowlight } from 'lowlight';
import { FileViewer } from 'react-file-viewer-v2';
import { media } from '@/api/endpoints';
import { ApiError } from '@/api/client';
import { showError, showSuccess } from '@/components/Toast';
import { confirm } from '@/components/ConfirmDialog';
import type { MediaFile } from '@/types';
import styles from './MediaLibrary.module.css';

const { Title } = Typography;

type ViewMode = 'grid' | 'list';
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

const lowlight = createLowlight(common);

const officeExts = new Set(['docx', 'xlsx', 'pptx', 'doc', 'xls', 'ppt']);

function getOfficeFileType(mime: string): string | undefined {
  if (!isOfficeMime(mime)) return undefined;
  if (mime.includes('wordprocessingml') || mime.includes('msword')) return 'docx';
  if (mime.includes('spreadsheetml') || mime.includes('ms-excel')) return 'xlsx';
  if (mime.includes('presentation') || mime.includes('ms-powerpoint')) return 'pptx';
  return undefined;
}

function getOfficeExt(filename: string): string | undefined {
  const ext = filename.split('.').pop()?.toLowerCase();
  return ext && officeExts.has(ext) ? ext : undefined;
}

function isOfficeMime(mime: string): boolean {
  return officeMimes.has(mime);
}

function isOfficeFile(mf: MediaFile): boolean {
  return isOfficeMime(mf.mime_type) || !!getOfficeExt(mf.filename);
}

const officeMimes = new Set([
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'application/vnd.openxmlformats-officedocument.presentationml.presentation',
  'application/msword',
  'application/vnd.ms-excel',
  'application/vnd.ms-powerpoint',
]);

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

function getCategoryIcon(category: FileCategory) {
  switch (category) {
    case 'image': return <Image size={16} />;
    case 'video': return <Film size={16} />;
    case 'audio': return <Music size={16} />;
    case 'document': return <FileText size={16} />;
    default: return <File size={16} />;
  }
}

function isPreviewable(mf: MediaFile): boolean {
  const cat = getFileCategory(mf.mime_type);
  return cat === 'image' || cat === 'video' || cat === 'audio' || mf.mime_type.startsWith('text/') || mf.mime_type === 'application/json' || mf.mime_type === 'application/pdf' || isOfficeFile(mf);
}

function guessLang(filename: string): string | undefined {
  const ext = filename.split('.').pop()?.toLowerCase();
  if (!ext) return undefined;
  const map: Record<string, string> = {
    ts: 'typescript', tsx: 'typescript', js: 'javascript', jsx: 'javascript',
    py: 'python', rb: 'ruby', go: 'go', rs: 'rust', java: 'java',
    c: 'c', cpp: 'cpp', h: 'c', hpp: 'cpp', cs: 'csharp',
    html: 'html', htm: 'html', css: 'css', scss: 'scss', less: 'less',
    json: 'json', xml: 'xml', yaml: 'yaml', yml: 'yaml', toml: 'ini',
    md: 'markdown', sql: 'sql', sh: 'bash', bash: 'bash', zsh: 'bash',
    php: 'php', swift: 'swift', kt: 'kotlin', scala: 'scala',
    r: 'r', lua: 'lua', vim: 'vim', dockerfile: 'dockerfile', makefile: 'makefile',
  };
  return map[ext];
}

function highlightCode(code: string, filename: string): string {
  const lang = guessLang(filename);
  try {
    const tree = lang
      ? lowlight.highlight(lang, code)
      : lowlight.highlightAuto(code);
    const html = tree.children
      .map((node) => renderNode(node))
      .join('');
    return html;
  } catch {
    return escapeHtml(code);
  }
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function renderNode(node: any): string {
  if (node.type === 'text') return escapeHtml(node.value ?? '');
  if (node.children) {
    const cls = node.properties?.className?.length
      ? `hljs-${node.properties.className[0]}`
      : '';
    const inner = node.children.map((c: any) => renderNode(c)).join('');
    return cls ? `<span class="${cls}">${inner}</span>` : inner;
  }
  return '';
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

const categories: { key: FileCategory; labelKey: string }[] = [
  { key: 'all', labelKey: 'common.all' },
  { key: 'image', labelKey: 'media.images' },
  { key: 'document', labelKey: 'media.documents' },
  { key: 'video', labelKey: 'media.videos' },
  { key: 'audio', labelKey: 'media.audio' },
  { key: 'other', labelKey: 'media.other' },
];

export default function MediaLibrary() {
  const { t } = useTranslation();
  const [files, setFiles] = useState<MediaFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState<FileCategory>('all');
  const [viewMode, setViewMode] = useState<ViewMode>(
    () => (localStorage.getItem('media-view-mode') as ViewMode) || 'grid',
  );
  const [uploads, setUploads] = useState<UploadProgress[]>([]);
  const [dragActive, setDragActive] = useState(false);
  const [previewFile, setPreviewFile] = useState<MediaFile | null>(null);
  const [copied, setCopied] = useState(false);
  const [textContent, setTextContent] = useState<string | null>(null);
  const [officeBlob, setOfficeBlob] = useState<Blob | null>(null);
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
    if (previewFile && (previewFile.mime_type.startsWith('text/') || previewFile.mime_type === 'application/json')) {
      fetch(previewFile.url)
        .then((r) => r.text())
        .then(setTextContent)
        .catch(() => setTextContent('Failed to load text content'));
    } else {
      setTextContent(null);
    }
  }, [previewFile]);

  useEffect(() => {
    const ft = previewFile ? getOfficeFileType(previewFile.mime_type) : undefined;
    if (ft && previewFile) {
      setOfficeBlob(null);
      fetch(previewFile.url)
        .then((r) => r.blob())
        .then(setOfficeBlob)
        .catch(() => setOfficeBlob(null));
    } else {
      setOfficeBlob(null);
    }
  }, [previewFile]);

  useEffect(() => {
    fetchFiles();
  }, [fetchFiles]);

  const filteredFiles = category === 'all'
    ? files
    : files.filter((f) => getFileCategory(f.mime_type) === category);

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
          if (previewFile?.id === mf.id) setPreviewFile(null);
        } catch (err) {
          if (err instanceof ApiError) {
            showError(err.message);
          }
        }
      },
    });
  };

  const handleCopyUrl = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      showSuccess(t('common.copied'));
      setTimeout(() => setCopied(false), 2000);
    } catch {
      showError(t('common.error_occurred'));
    }
  };

  const totalPages = Math.ceil(total / perPage);

  const columns = [
    {
      title: '',
      key: 'thumb',
      width: 70,
      render: (_: unknown, record: MediaFile) => {
        const cat = getFileCategory(record.mime_type);
        return cat === 'image' ? (
          <img
            src={record.url}
            alt=""
            className={styles.thumbCell}
            onClick={() => setPreviewFile(record)}
            style={{ cursor: 'pointer' }}
          />
        ) : (
          <div className={styles.thumbCellPlaceholder}>{getCategoryIcon(cat)}</div>
        );
      },
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
      width: 100,
      render: (_: unknown, record: MediaFile) => (
        <div style={{ display: 'flex', gap: 4 }}>
          {isPreviewable(record) && (
            <button
              type="button"
              className={styles.actionBtn}
              onClick={() => setPreviewFile(record)}
              title={t('common.preview')}
            >
              <Image size={14} />
            </button>
          )}
          <button
            type="button"
            className={styles.deleteBtn}
            onClick={() => handleDelete(record)}
          >
            <Trash2 size={14} />
          </button>
        </div>
      ),
    },
  ];

  const isLargePreview = (mf: MediaFile) =>
    mf.mime_type === 'application/pdf' ||
    isOfficeMime(mf.mime_type) ||
    mf.mime_type.startsWith('text/') ||
    mf.mime_type === 'application/json';

  const renderPreview = () => {
    if (!previewFile) return null;
    const cat = getFileCategory(previewFile.mime_type);
    const officeFileType = getOfficeFileType(previewFile.mime_type);
    const large = isLargePreview(previewFile);

    return (
      <div className={styles.previewOverlay} onClick={() => setPreviewFile(null)}>
        <div
          className={`${styles.previewModal} ${large ? styles.previewModalLarge : ''}`}
          onClick={(e) => e.stopPropagation()}
        >
          <div className={styles.previewHeader}>
            <div className={styles.previewTitle}>
              {getCategoryIcon(cat)}
              <span>{previewFile.filename}</span>
            </div>
            <div className={styles.previewActions}>
              <button
                type="button"
                className={styles.previewActionBtn}
                onClick={() => handleCopyUrl(previewFile.url)}
                title={t('media.copy_url')}
              >
                {copied ? <Check size={16} /> : <Copy size={16} />}
              </button>
              <button
                type="button"
                className={styles.previewActionBtn}
                onClick={() => handleDelete(previewFile)}
                title={t('common.delete')}
              >
                <Trash2 size={16} />
              </button>
              <button
                type="button"
                className={styles.previewCloseBtn}
                onClick={() => setPreviewFile(null)}
              >
                <X size={18} />
              </button>
            </div>
          </div>
          <div className={styles.previewBody}>
            {cat === 'image' && (
              <img src={previewFile.url} alt={previewFile.filename} className={styles.previewImage} />
            )}
            {cat === 'video' && (
              <video src={previewFile.url} controls className={styles.previewMedia} />
            )}
            {cat === 'audio' && (
              <div className={styles.previewAudioWrap}>
                <Music size={48} style={{ color: 'var(--color-primary)', marginBottom: '1rem' }} />
                <audio src={previewFile.url} controls />
              </div>
            )}
            {previewFile.mime_type === 'application/pdf' && (
              <iframe src={previewFile.url} className={styles.previewIframe} title={previewFile.filename} />
            )}
            {officeFileType && officeBlob && (
              <div className={styles.previewOfficeWrap}>
                <FileViewer file={officeBlob} fileType={officeFileType} />
              </div>
            )}
            {isOfficeMime(previewFile.mime_type) && !officeFileType && (
              <div className={styles.previewAudioWrap}>
                <FileText size={48} style={{ color: 'var(--color-text-tertiary)', marginBottom: '1rem' }} />
                <span style={{ color: 'var(--color-text-secondary)', fontSize: '0.9375rem', marginBottom: '0.5rem' }}>
                  {previewFile.filename}
                </span>
                <span style={{ color: 'var(--color-text-tertiary)', fontSize: '0.8125rem', marginBottom: '1rem' }}>
                  {t('media.preview_not_supported', { defaultValue: 'Preview not supported for this format' })}
                </span>
                <a
                  href={previewFile.url}
                  download={previewFile.filename}
                  className={styles.downloadLink}
                >
                  <Download size={16} />
                  {t('media.download', { defaultValue: 'Download' })}
                </a>
              </div>
            )}
            {(previewFile.mime_type.startsWith('text/') || previewFile.mime_type === 'application/json') && textContent !== null && (
              <pre
                className={styles.previewCode}
                dangerouslySetInnerHTML={{ __html: highlightCode(textContent, previewFile.filename) }}
              />
            )}
            {!['image', 'video', 'audio'].includes(cat) && previewFile.mime_type !== 'application/pdf' && !isOfficeMime(previewFile.mime_type) && !previewFile.mime_type.startsWith('text/') && previewFile.mime_type !== 'application/json' && (
              <div className={styles.previewAudioWrap}>
                <File size={48} style={{ color: 'var(--color-text-tertiary)', marginBottom: '1rem' }} />
                <span style={{ color: 'var(--color-text-tertiary)', fontSize: '0.875rem' }}>
                  {previewFile.mime_type}
                </span>
              </div>
            )}
          </div>
          <div className={styles.previewMeta}>
            <span>{previewFile.mime_type}</span>
            <span>{formatFileSize(previewFile.size)}</span>
            {previewFile.width && previewFile.height && (
              <span>{previewFile.width}×{previewFile.height}</span>
            )}
          </div>
        </div>
      </div>
    );
  };

  const renderMediaThumb = (mf: MediaFile) => {
    const cat = getFileCategory(mf.mime_type);
    if (cat === 'image') {
      return (
        <img
          src={mf.url}
          alt={mf.filename}
          className={styles.mediaThumb}
          onClick={() => setPreviewFile(mf)}
          style={{ cursor: 'pointer' }}
        />
      );
    }
    return (
      <div
        className={styles.mediaThumbPlaceholder}
        onClick={() => isPreviewable(mf) ? setPreviewFile(mf) : undefined}
        style={isPreviewable(mf) ? { cursor: 'pointer' } : undefined}
      >
        {cat === 'video' && <Film size={32} />}
        {cat === 'audio' && <Music size={32} />}
        {cat === 'document' && <FileText size={32} />}
        {cat === 'other' && <File size={32} />}
        <span className={styles.mediaThumbExt}>
          {mf.filename.split('.').pop()?.toUpperCase() || 'FILE'}
        </span>
      </div>
    );
  };

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
        <div className={styles.dropzoneHint}>PNG, JPG, GIF, SVG, PDF, CSV, etc.</div>
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

      <div className={styles.toolbar}>
        <div className={styles.categoryTabs}>
          {categories.map((c) => (
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
            prefix={<Search size={16} />}
            placeholder={t('common.search')}
            value={search}
            onChange={setSearch}
            allowClear
            onClear={() => { setSearch(''); setPage(1); }}
            onKeyDown={(e) => { if (e.key === 'Enter') { setPage(1); fetchFiles(); } }}
          />
        </div>
      </div>

      {loading ? (
        <div className={styles.loadingWrap}><Spin size={40} /></div>
      ) : error ? (
        <div className={styles.emptyState}>
          <p>{t('common.error_occurred')}</p>
          <Button onClick={fetchFiles}>{t('common.try_again')}</Button>
        </div>
      ) : filteredFiles.length === 0 ? (
        <div className={styles.emptyState}>
          <div className={styles.emptyIcon}><Image size={32} /></div>
          <div className={styles.emptyTitle}>{t('media.no_files')}</div>
        </div>
      ) : viewMode === 'grid' ? (
        <>
          <div className={styles.gridView}>
            {filteredFiles.map((mf) => (
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
                {renderMediaThumb(mf)}
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
              data={filteredFiles}
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

      {createPortal(renderPreview(), document.body)}
    </div>
  );
}

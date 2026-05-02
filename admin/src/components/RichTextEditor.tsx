import { useCallback, useEffect, useState, useRef } from 'react';
import { useEditor, EditorContent } from '@tiptap/react';
import { BubbleMenu } from '@tiptap/react/menus';
import { Node } from '@tiptap/core';
import { ReactNodeViewRenderer, NodeViewWrapper } from '@tiptap/react';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import StarterKit from '@tiptap/starter-kit';
import Underline from '@tiptap/extension-underline';
import Highlight from '@tiptap/extension-highlight';
import ImageExt from '@tiptap/extension-image';
import Link from '@tiptap/extension-link';
import { Table, TableRow, TableCell, TableHeader } from '@tiptap/extension-table';
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
import Placeholder from '@tiptap/extension-placeholder';
import { common, createLowlight } from 'lowlight';
import { useTranslation } from 'react-i18next';
import {
  Bold,
  Italic,
  Strikethrough,
  Heading1,
  Heading2,
  Heading3,
  List,
  ListOrdered,
  Quote,
  Code,
  Link as LinkIcon,
  ImageIcon,
  TableIcon,
  Minus,
  Undo2,
  Redo2,
  Highlighter,
  Pilcrow,
  ArrowRight,
  Underline as UnderlineIcon,
  Paperclip,
  Film,
  Music,
  File,
  FileText,
  Download,
  X,
  Eye,
  EyeOff,
} from 'lucide-react';
import { FileViewer } from 'react-file-viewer-v2';
import MediaPicker from './MediaPicker';
import type { MediaFile } from '@/types';
import styles from './RichTextEditor.module.css';

const lowlight = createLowlight(common);

/* ── Helpers ── */

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function getFileCategory(mime: string): string {
  if (mime.startsWith('image/')) return 'image';
  if (mime.startsWith('video/')) return 'video';
  if (mime.startsWith('audio/')) return 'audio';
  if (mime.startsWith('text/') || mime === 'application/pdf' || mime === 'application/json') return 'document';
  return 'other';
}

function getFileIcon(category: string, size = 20) {
  switch (category) {
    case 'image': return <ImageIcon size={size} />;
    case 'video': return <Film size={size} />;
    case 'audio': return <Music size={size} />;
    case 'document': return <FileText size={size} />;
    default: return <File size={size} />;
  }
}

const officeMimes = new Set([
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'application/vnd.openxmlformats-officedocument.presentationml.presentation',
  'application/msword',
  'application/vnd.ms-excel',
  'application/vnd.ms-powerpoint',
]);

function isPreviewableDoc(mime: string): boolean {
  return mime === 'application/pdf' || officeMimes.has(mime);
}

function getOfficeViewerType(mime: string): string | undefined {
  if (mime.includes('wordprocessingml') || mime.includes('msword')) return 'docx';
  if (mime.includes('spreadsheetml') || mime.includes('ms-excel')) return 'xlsx';
  if (mime.includes('presentation') || mime.includes('ms-powerpoint')) return 'pptx';
  return undefined;
}

/* ── Direct node insertion ── */

function insertNodeOfType(ed: any, typeName: string) {
  const nodeType = ed.state.schema.nodes[typeName];
  if (!nodeType) return;
  ed.chain().command(({ tr }: any) => {
    tr.replaceSelectionWith(nodeType.create());
    return true;
  }).run();
}

/* ═══════════════════════════════════════════════════════════
   Custom Image extension with width support
   ═══════════════════════════════════════════════════════════ */

const CustomImage = ImageExt.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      width: {
        default: null,
        parseHTML: (el: HTMLElement) => el.style.width || null,
        renderHTML: (attrs: any) => {
          if (!attrs.width) return {};
          return { style: `width: ${attrs.width}` };
        },
      },
    };
  },
});

/* ═══════════════════════════════════════════════════════════
   Custom TipTap Nodes
   ═══════════════════════════════════════════════════════════ */

/* ── ImagePlaceholder ── */

function ImagePlaceholderView({ editor, node, getPos }: any) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<'buttons' | 'url'>('buttons');
  const [url, setUrl] = useState('');
  const [pickerOpen, setPickerOpen] = useState(false);

  const replaceWithImage = useCallback((src: string) => {
    const pos = getPos();
    if (pos === undefined) return;
    const sanitized = sanitizeUrl(src, 'image');
    if (!sanitized) return;
    editor.chain().focus().command(({ tr }: any) => {
      const imgNode = editor.schema.nodes.image.create({ src: sanitized });
      tr.replaceWith(pos, pos + node.nodeSize, imgNode);
      return true;
    }).run();
  }, [editor, node, getPos]);

  const handleUrlSubmit = () => {
    if (url.trim()) replaceWithImage(url.trim());
  };

  const handleMediaSelect = useCallback((file: MediaFile) => {
    setPickerOpen(false);
    replaceWithImage(file.url);
  }, [replaceWithImage]);

  return (
    <NodeViewWrapper>
      <div className={styles.placeholderCard} contentEditable={false}>
        {mode === 'buttons' ? (
          <div className={styles.placeholderContent}>
            <ImageIcon size={32} className={styles.placeholderIcon} />
            <span className={styles.placeholderLabel}>{t('editor.insert_image')}</span>
            <div className={styles.placeholderActions}>
              <button type="button" className={styles.placeholderBtn} onClick={() => setMode('url')}>
                <LinkIcon size={14} />
                {t('editor.enter_url')}
              </button>
              <button type="button" className={`${styles.placeholderBtn} ${styles.placeholderBtnPrimary}`} onClick={() => setPickerOpen(true)}>
                <ImageIcon size={14} />
                {t('editor.from_library')}
              </button>
            </div>
          </div>
        ) : (
          <div className={styles.placeholderUrlInput}>
            <LinkIcon size={14} className={styles.urlInputIcon} />
            <input
              className={styles.urlInputField}
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleUrlSubmit();
                if (e.key === 'Escape') setMode('buttons');
              }}
              placeholder="https://example.com/image.jpg"
              autoFocus
            />
            <button type="button" className={styles.urlInputConfirm} onClick={handleUrlSubmit}>
              <ArrowRight size={14} />
            </button>
            <button type="button" className={styles.urlInputCancel} onClick={() => setMode('buttons')}>
              <X size={14} />
            </button>
          </div>
        )}
      </div>
      <MediaPicker open={pickerOpen} onSelect={handleMediaSelect} onClose={() => setPickerOpen(false)} filter="image" />
    </NodeViewWrapper>
  );
}

const ImagePlaceholderExtension = Node.create({
  name: 'imagePlaceholder',
  group: 'block',
  atom: true,
  selectable: true,
  draggable: false,
  parseHTML() { return [{ tag: 'div[data-image-placeholder]' }]; },
  renderHTML() { return ['div', { 'data-image-placeholder': '', style: 'display:none' }]; },
  addNodeView() { return ReactNodeViewRenderer(ImagePlaceholderView); },
});

/* ── FilePlaceholder ── */

function FilePlaceholderView({ editor, node, getPos }: any) {
  const { t } = useTranslation();
  const [pickerOpen, setPickerOpen] = useState(false);

  const handleSelect = useCallback((file: MediaFile) => {
    setPickerOpen(false);
    const pos = getPos();
    if (pos === undefined) return;
    editor.chain().focus().command(({ tr }: any) => {
      const fileNode = editor.schema.nodes.fileEmbed.create({
        url: file.url,
        filename: file.filename,
        filesize: file.size,
        mimetype: file.mime_type,
      });
      tr.replaceWith(pos, pos + node.nodeSize, fileNode);
      return true;
    }).run();
  }, [editor, node, getPos]);

  return (
    <NodeViewWrapper>
      <div className={styles.placeholderCard} contentEditable={false}>
        <div className={styles.placeholderContent}>
          <Paperclip size={32} className={styles.placeholderIcon} />
          <span className={styles.placeholderLabel}>{t('editor.insert_file')}</span>
          <div className={styles.placeholderActions}>
            <button type="button" className={`${styles.placeholderBtn} ${styles.placeholderBtnPrimary}`} onClick={() => setPickerOpen(true)}>
              <File size={14} />
              {t('editor.from_library')}
            </button>
          </div>
        </div>
      </div>
      <MediaPicker open={pickerOpen} onSelect={handleSelect} onClose={() => setPickerOpen(false)} />
    </NodeViewWrapper>
  );
}

const FilePlaceholderExtension = Node.create({
  name: 'filePlaceholder',
  group: 'block',
  atom: true,
  selectable: true,
  draggable: false,
  parseHTML() { return [{ tag: 'div[data-file-placeholder]' }]; },
  renderHTML() { return ['div', { 'data-file-placeholder': '', style: 'display:none' }]; },
  addNodeView() { return ReactNodeViewRenderer(FilePlaceholderView); },
});

/* ── FileEmbed with inline preview ── */

function FileEmbedView({ node }: any) {
  const { t } = useTranslation();
  const { filename, filesize, mimetype, url } = node.attrs;
  const [previewOpen, setPreviewOpen] = useState(false);
  const [officeBlob, setOfficeBlob] = useState<Blob | null>(null);

  const cat = getFileCategory(mimetype || '');
  const ext = filename ? filename.split('.').pop()?.toUpperCase() || 'FILE' : 'FILE';
  const canPreview = isPreviewableDoc(mimetype || '');
  const isPdf = mimetype === 'application/pdf';
  const officeType = getOfficeViewerType(mimetype || '');

  useEffect(() => {
    if (!previewOpen || !officeType || !url) return;
    let cancelled = false;
    fetch(url)
      .then((r) => r.blob())
      .then((blob) => { if (!cancelled) setOfficeBlob(blob); })
      .catch(() => { if (!cancelled) setOfficeBlob(null); });
    return () => { cancelled = true; };
  }, [previewOpen, officeType, url]);

  return (
    <NodeViewWrapper>
      <div contentEditable={false}>
        <div className={styles.fileEmbed}>
          <a
            className={styles.fileEmbedMain}
            href={url}
            download={filename || true}
            target="_blank"
            rel="noopener noreferrer"
            onClick={(e) => e.stopPropagation()}
          >
            <div className={styles.fileEmbedIcon}>{getFileIcon(cat)}</div>
            <div className={styles.fileEmbedInfo}>
              <span className={styles.fileEmbedName}>{filename || 'File'}</span>
              <span className={styles.fileEmbedMeta}>
                {ext}{filesize ? ` · ${formatFileSize(filesize)}` : ''}
              </span>
            </div>
            <Download size={16} className={styles.fileEmbedDownload} />
          </a>
          {canPreview && (
            <button
              type="button"
              className={styles.filePreviewToggle}
              onClick={(e) => { e.stopPropagation(); setPreviewOpen(!previewOpen); }}
              title={previewOpen ? t('editor.close_preview') : t('editor.preview')}
            >
              {previewOpen ? <EyeOff size={14} /> : <Eye size={14} />}
            </button>
          )}
        </div>
        {previewOpen && (
          <div className={styles.filePreviewArea}>
            {isPdf && (
              <iframe src={url} className={styles.filePreviewFrame} title={filename} />
            )}
            {officeType && officeBlob && (
              <div className={styles.filePreviewOffice}>
                <FileViewer file={officeBlob} fileType={officeType} />
              </div>
            )}
            {officeType && !officeBlob && (
              <div className={styles.filePreviewLoading}>{t('common.loading')}</div>
            )}
          </div>
        )}
      </div>
    </NodeViewWrapper>
  );
}

const noRender = () => ({});

const FileEmbedExtension = Node.create({
  name: 'fileEmbed',
  group: 'block',
  atom: true,
  selectable: true,

  addAttributes() {
    return {
      url:     { default: null,  parseHTML: (el: HTMLElement) => el.getAttribute('href'),                         renderHTML: noRender },
      filename: { default: '',   parseHTML: (el: HTMLElement) => el.getAttribute('data-filename') || el.textContent || '', renderHTML: noRender },
      filesize: { default: 0,    parseHTML: (el: HTMLElement) => parseInt(el.getAttribute('data-filesize') || '0', 10),    renderHTML: noRender },
      mimetype: { default: '',   parseHTML: (el: HTMLElement) => el.getAttribute('data-mimetype') || '',                  renderHTML: noRender },
    };
  },

  parseHTML() { return [{ tag: 'a[data-file-embed]' }]; },

  renderHTML({ node }: any) {
    return ['a', {
      'data-file-embed': '',
      href: node.attrs.url || '',
      download: node.attrs.filename || '',
      target: '_blank',
      rel: 'noopener noreferrer',
      'data-filename': node.attrs.filename || '',
      'data-filesize': String(node.attrs.filesize || 0),
      'data-mimetype': node.attrs.mimetype || '',
    }, node.attrs.filename || 'File'];
  },

  addNodeView() { return ReactNodeViewRenderer(FileEmbedView); },
});

/* ═══════════════════════════════════════════════════════════
   URL Popover (links only)
   ═══════════════════════════════════════════════════════════ */

interface UrlPopoverState {
  open: boolean;
  url: string;
  top: number;
  left: number;
}

function UrlInputPopover({ state, onConfirm, onCancel }: {
  state: UrlPopoverState;
  onConfirm: (url: string) => void;
  onCancel: () => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [localUrl, setLocalUrl] = useState('');

  useEffect(() => {
    if (state.open) {
      setLocalUrl('');
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [state.open]);

  if (!state.open) return null;

  const submit = () => {
    if (localUrl.trim()) {
      onConfirm(localUrl.trim());
      onCancel();
    }
  };

  return (
    <div className={styles.urlPopover} style={{ top: state.top, left: state.left }}>
      <span className={styles.urlPopoverIcon}><LinkIcon size={14} /></span>
      <input
        ref={inputRef}
        className={styles.urlPopoverInput}
        value={localUrl}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => setLocalUrl(e.target.value)}
        onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); submit(); } if (e.key === 'Escape') onCancel(); }}
        placeholder="Paste or type a link…"
        type="url"
      />
      <button type="button" className={styles.urlPopoverBtn} onClick={submit}>
        <ArrowRight size={14} />
      </button>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════
   Slash Command Menu
   ═══════════════════════════════════════════════════════════ */

interface SlashItem {
  title: string;
  description: string;
  icon: React.ReactNode;
  command: (editor: any) => void;
}

const SLASH_ITEMS: SlashItem[] = [
  { title: 'Text', description: 'Plain text block', icon: <Pilcrow size={18} />, command: (ed) => ed.chain().focus().setParagraph().run() },
  { title: 'Heading 1', description: 'Large heading', icon: <Heading1 size={18} />, command: (ed) => ed.chain().focus().toggleHeading({ level: 1 }).run() },
  { title: 'Heading 2', description: 'Medium heading', icon: <Heading2 size={18} />, command: (ed) => ed.chain().focus().toggleHeading({ level: 2 }).run() },
  { title: 'Heading 3', description: 'Small heading', icon: <Heading3 size={18} />, command: (ed) => ed.chain().focus().toggleHeading({ level: 3 }).run() },
  { title: 'Bullet List', description: 'Unordered list', icon: <List size={18} />, command: (ed) => ed.chain().focus().toggleBulletList().run() },
  { title: 'Numbered List', description: 'Ordered list', icon: <ListOrdered size={18} />, command: (ed) => ed.chain().focus().toggleOrderedList().run() },
  { title: 'Image', description: 'Embed an image', icon: <ImageIcon size={18} />, command: (ed) => insertNodeOfType(ed, 'imagePlaceholder') },
  { title: 'File', description: 'Attach a file', icon: <Paperclip size={18} />, command: (ed) => insertNodeOfType(ed, 'filePlaceholder') },
  { title: 'Link', description: 'Insert a hyperlink', icon: <LinkIcon size={18} />, command: () => {} },
  { title: 'Table', description: '3×3 table', icon: <TableIcon size={18} />, command: (ed) => ed.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run() },
  { title: 'Quote', description: 'Block quote', icon: <Quote size={18} />, command: (ed) => ed.chain().focus().toggleBlockquote().run() },
  { title: 'Code Block', description: 'Syntax-highlighted code', icon: <Code size={18} />, command: (ed) => ed.chain().focus().toggleCodeBlock().run() },
  { title: 'Divider', description: 'Horizontal rule', icon: <Minus size={18} />, command: (ed) => ed.chain().focus().setHorizontalRule().run() },
];

const slashPluginKey = new PluginKey('slashCommand');

function SlashMenu({ editor }: { editor: any }) {
  const [, forceRender] = useState(0);
  const stateRef = useRef({ open: false, query: '', selected: 0, top: 0, left: 0 });
  const menuRef = useRef<HTMLDivElement>(null);

  const rerender = useCallback(() => forceRender(n => n + 1), []);

  const getFiltered = useCallback((q: string) =>
    SLASH_ITEMS.filter(
      (item) => item.title.toLowerCase().includes(q.toLowerCase()) || item.description.toLowerCase().includes(q.toLowerCase())
    ), []);

  const close = useCallback(() => {
    const s = stateRef.current;
    if (!s.open) return;
    s.open = false;
    s.query = '';
    s.selected = 0;
    rerender();
  }, [rerender]);

  const execute = useCallback((item: SlashItem) => {
    // Read query directly from document instead of potentially-stale ref
    const { from } = editor.state.selection;
    const textBefore = editor.state.doc.textBetween(Math.max(0, from - 30), from, '\n');
    const match = textBefore.match(/\/([^\s]*)$/);
    if (!match) { close(); return; }
    const slashPos = from - match[1].length - 1;

    // Single transaction: delete slash text + insert node
    if (item.title === 'Image' || item.title === 'File') {
      const typeName = item.title === 'Image' ? 'imagePlaceholder' : 'filePlaceholder';
      const nodeType = editor.state.schema.nodes[typeName];
      if (nodeType) {
        editor.chain()
          .deleteRange({ from: slashPos, to: from })
          .command(({ tr }: any) => {
            tr.replaceSelectionWith(nodeType.create());
            return true;
          })
          .run();
      }
    } else {
      editor.chain().deleteRange({ from: slashPos, to: from }).run();
      item.command(editor);
    }
    close();
  }, [editor, close]);

  useEffect(() => {
    if (!editor) return;

    const handleKeyDown = (_view: any, event: KeyboardEvent) => {
      const s = stateRef.current;
      if (!s.open) return false;
      if (event.key === 'Escape') { close(); return true; }
      if (event.key === 'ArrowDown') {
        event.preventDefault();
        const filtered = getFiltered(s.query);
        s.selected = (s.selected + 1) % filtered.length;
        rerender();
        return true;
      }
      if (event.key === 'ArrowUp') {
        event.preventDefault();
        const filtered = getFiltered(s.query);
        s.selected = (s.selected - 1 + filtered.length) % filtered.length;
        rerender();
        return true;
      }
      if (event.key === 'Enter') {
        event.preventDefault();
        const filtered = getFiltered(s.query);
        if (filtered[s.selected]) execute(filtered[s.selected]);
        return true;
      }
      return false;
    };

    const plugin = new Plugin({
      key: slashPluginKey,
      handleKeyDown,
      view() {
        return {
          update(view) {
            const { from } = view.state.selection;
            const textBefore = view.state.doc.textBetween(Math.max(0, from - 30), from, '\n');
            const match = textBefore.match(/\/([^\s]*)$/);
            const s = stateRef.current;
            if (match) {
              s.query = match[1];
              s.selected = 0;
              if (!s.open) {
                const coords = view.coordsAtPos(from);
                const editorRect = view.dom.parentElement?.getBoundingClientRect();
                s.top = coords.bottom - (editorRect?.top ?? 0) + 8;
                s.left = coords.left - (editorRect?.left ?? 0);
              }
              s.open = true;
              rerender();
            } else if (s.open) {
              close();
            }
          },
          destroy() {},
        };
      },
    });

    editor.registerPlugin(plugin);
    return () => { editor.unregisterPlugin(slashPluginKey); };
  }, [editor, close, execute, getFiltered, rerender]);

  useEffect(() => {
    if (!menuRef.current) return;
    const active = menuRef.current.querySelector(`[data-active="true"]`);
    active?.scrollIntoView({ block: 'nearest' });
  });

  const s = stateRef.current;
  if (!s.open) return null;

  const filtered = getFiltered(s.query);
  if (filtered.length === 0) return null;

  return (
    <div
      ref={menuRef}
      className={styles.slashMenu}
      style={{ top: s.top, left: s.left }}
    >
      <div className={styles.slashMenuLabel}>Blocks</div>
      {filtered.map((item, idx) => (
        <button
          key={item.title}
          type="button"
          data-active={idx === s.selected}
          className={`${styles.slashMenuItem} ${idx === s.selected ? styles.slashMenuItemActive : ''}`}
          onMouseDown={(e) => e.preventDefault()} /* prevent editor blur */
          onClick={() => execute(item)}
          onMouseEnter={() => { s.selected = idx; rerender(); }}
        >
          <span className={styles.slashMenuIcon}>{item.icon}</span>
          <span className={styles.slashMenuText}>
            <span className={styles.slashMenuTitle}>{item.title}</span>
            <span className={styles.slashMenuDesc}>{item.description}</span>
          </span>
        </button>
      ))}
    </div>
  );
}

/* ── URL sanitization ── */

function sanitizeUrl(url: string, mode: 'link' | 'image'): string | null {
  if (url.startsWith('/') || url.startsWith('#')) return url;
  try {
    const parsed = new URL(url);
    const allowed = mode === 'image' ? ['http:', 'https:', 'data:'] : ['http:', 'https:', 'mailto:', 'tel:'];
    if (!allowed.includes(parsed.protocol)) return null;
    return url;
  } catch {
    return url;
  }
}

/* ═══════════════════════════════════════════════════════════
   RichTextEditor Component
   ═══════════════════════════════════════════════════════════ */

interface RichTextEditorProps {
  value: string;
  onChange: (html: string) => void;
  placeholder?: string;
  notion?: boolean;
}

const IMAGE_SIZES = ['100%', '50%', '25%', '20%'];

export default function RichTextEditor({ value, onChange, placeholder, notion }: RichTextEditorProps) {
  const contentClass = notion ? `${styles.editorContent} ${styles.editorNotion}` : styles.editorContent;

  const editor = useEditor({
    extensions: [
      StarterKit.configure({ codeBlock: false }),
      Underline,
      Highlight.configure({ multicolor: false }),
      CustomImage.configure({ inline: false }),
      Link.configure({ openOnClick: false, autolink: true }),
      Table.configure({ resizable: false }),
      TableRow,
      TableCell,
      TableHeader,
      CodeBlockLowlight.configure({ lowlight }),
      Placeholder.configure({ placeholder: placeholder || '' }),
      ImagePlaceholderExtension,
      FilePlaceholderExtension,
      FileEmbedExtension,
    ],
    content: value,
    onUpdate: ({ editor: e }) => onChange(e.getHTML()),
    editorProps: { attributes: { class: contentClass } },
  });

  useEffect(() => {
    if (editor && value !== editor.getHTML()) {
      editor.commands.setContent(value);
    }
  }, [value, editor]);

  const [urlPopover, setUrlPopover] = useState<UrlPopoverState>({ open: false, url: '', top: 0, left: 0 });

  const openUrlPopover = useCallback(() => {
    if (!editor) return;
    const { from } = editor.state.selection;
    const coords = editor.view.coordsAtPos(from);
    const rect = editor.view.dom.parentElement?.getBoundingClientRect();
    setUrlPopover({ open: true, url: '', top: coords.bottom - (rect?.top ?? 0) + 8, left: coords.left - (rect?.left ?? 0) });
  }, [editor]);

  const handleUrlConfirm = useCallback((url: string) => {
    if (!editor || !url) return;
    const sanitized = sanitizeUrl(url.trim(), 'link');
    if (!sanitized) return;
    editor.chain().focus().extendMarkRange('link').setLink({ href: sanitized }).run();
  }, [editor]);

  const handleUrlCancel = useCallback(() => {
    setUrlPopover((s) => ({ ...s, open: false }));
    editor?.commands.focus();
  }, [editor]);

  const insertNode = useCallback((typeName: string) => {
    if (!editor) return;
    insertNodeOfType(editor, typeName);
  }, [editor]);

  if (!editor) return null;

  type ToolbarBtn = { icon: React.ReactNode; action: () => void; active?: boolean; title: string };

  const toolbarItems: (ToolbarBtn | string)[] = [
    { icon: <Bold size={16} />, action: () => editor.chain().focus().toggleBold().run(), active: editor.isActive('bold'), title: 'Bold' },
    { icon: <Italic size={16} />, action: () => editor.chain().focus().toggleItalic().run(), active: editor.isActive('italic'), title: 'Italic' },
    { icon: <Strikethrough size={16} />, action: () => editor.chain().focus().toggleStrike().run(), active: editor.isActive('strike'), title: 'Strikethrough' },
    { icon: <Highlighter size={16} />, action: () => editor.chain().focus().toggleHighlight().run(), active: editor.isActive('highlight'), title: 'Highlight' },
    '|',
    { icon: <Heading1 size={16} />, action: () => editor.chain().focus().toggleHeading({ level: 1 }).run(), active: editor.isActive('heading', { level: 1 }), title: 'Heading 1' },
    { icon: <Heading2 size={16} />, action: () => editor.chain().focus().toggleHeading({ level: 2 }).run(), active: editor.isActive('heading', { level: 2 }), title: 'Heading 2' },
    { icon: <Heading3 size={16} />, action: () => editor.chain().focus().toggleHeading({ level: 3 }).run(), active: editor.isActive('heading', { level: 3 }), title: 'Heading 3' },
    '|',
    { icon: <List size={16} />, action: () => editor.chain().focus().toggleBulletList().run(), active: editor.isActive('bulletList'), title: 'Bullet List' },
    { icon: <ListOrdered size={16} />, action: () => editor.chain().focus().toggleOrderedList().run(), active: editor.isActive('orderedList'), title: 'Ordered List' },
    { icon: <Quote size={16} />, action: () => editor.chain().focus().toggleBlockquote().run(), active: editor.isActive('blockquote'), title: 'Blockquote' },
    { icon: <Code size={16} />, action: () => editor.chain().focus().toggleCodeBlock().run(), active: editor.isActive('codeBlock'), title: 'Code Block' },
    '|',
    { icon: <LinkIcon size={16} />, action: () => openUrlPopover(), active: editor.isActive('link'), title: 'Link' },
    { icon: <ImageIcon size={16} />, action: () => insertNode('imagePlaceholder'), title: 'Image' },
    { icon: <Paperclip size={16} />, action: () => insertNode('filePlaceholder'), title: 'File' },
    { icon: <TableIcon size={16} />, action: () => editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(), title: 'Table' },
    { icon: <Minus size={16} />, action: () => editor.chain().focus().setHorizontalRule().run(), title: 'Horizontal Rule' },
    '|',
    { icon: <Undo2 size={16} />, action: () => editor.chain().focus().undo().run(), title: 'Undo' },
    { icon: <Redo2 size={16} />, action: () => editor.chain().focus().redo().run(), title: 'Redo' },
  ];

  const bubbleItems: (ToolbarBtn | string)[] = [
    { icon: <Bold size={14} />, action: () => editor.chain().focus().toggleBold().run(), active: editor.isActive('bold'), title: 'Bold' },
    { icon: <Italic size={14} />, action: () => editor.chain().focus().toggleItalic().run(), active: editor.isActive('italic'), title: 'Italic' },
    { icon: <UnderlineIcon size={14} />, action: () => editor.chain().focus().toggleUnderline().run(), active: editor.isActive('underline'), title: 'Underline' },
    { icon: <Strikethrough size={14} />, action: () => editor.chain().focus().toggleStrike().run(), active: editor.isActive('strike'), title: 'Strike' },
    { icon: <Highlighter size={14} />, action: () => editor.chain().focus().toggleHighlight().run(), active: editor.isActive('highlight'), title: 'Highlight' },
    { icon: <Code size={14} />, action: () => editor.chain().focus().toggleCode().run(), active: editor.isActive('code'), title: 'Inline Code' },
    '|',
    { icon: <Heading1 size={14} />, action: () => editor.chain().focus().toggleHeading({ level: 1 }).run(), active: editor.isActive('heading', { level: 1 }), title: 'H1' },
    { icon: <Heading2 size={14} />, action: () => editor.chain().focus().toggleHeading({ level: 2 }).run(), active: editor.isActive('heading', { level: 2 }), title: 'H2' },
    { icon: <Heading3 size={14} />, action: () => editor.chain().focus().toggleHeading({ level: 3 }).run(), active: editor.isActive('heading', { level: 3 }), title: 'H3' },
    '|',
    { icon: <LinkIcon size={14} />, action: () => openUrlPopover(), active: editor.isActive('link'), title: 'Link' },
    { icon: <Quote size={14} />, action: () => editor.chain().focus().toggleBlockquote().run(), active: editor.isActive('blockquote'), title: 'Quote' },
    { icon: <List size={14} />, action: () => editor.chain().focus().toggleBulletList().run(), active: editor.isActive('bulletList'), title: 'Bullet List' },
  ];

  const renderItems = (items: (ToolbarBtn | string)[]) =>
    items.map((item, idx) => {
      if (typeof item === 'string') return <div key={`sep-${idx}`} className={styles.toolbarDivider} />;
      return (
        <button key={item.title} type="button" className={`${styles.toolbarBtn} ${item.active ? styles.toolbarBtnActive : ''}`} onClick={item.action} title={item.title} tabIndex={-1}>
          {item.icon}
        </button>
      );
    });

  const currentImageWidth = editor.getAttributes('image').width || null;

  const imageBubbleMenu = (
    <BubbleMenu editor={editor} className={styles.imageBubbleMenu} shouldShow={({ editor: ed }: any) => ed.isActive('image')}>
      <ImageIcon size={14} className={styles.imageBubbleIcon} />
      {IMAGE_SIZES.map((pct) => (
        <button
          key={pct}
          type="button"
          className={`${styles.imageSizeBtn} ${currentImageWidth === pct ? styles.imageSizeBtnActive : ''}`}
          onClick={() => editor.chain().focus().updateAttributes('image', { width: pct }).run()}
        >
          {pct}
        </button>
      ))}
    </BubbleMenu>
  );

  if (notion) {
    return (
      <div className={styles.editorNotionWrapper}>
        <BubbleMenu
          editor={editor}
          className={styles.bubbleMenu}
          shouldShow={({ editor: ed, state }: any) => {
            if (ed.isActive('image') || ed.isActive('imagePlaceholder') || ed.isActive('fileEmbed') || ed.isActive('filePlaceholder')) return false;
            const { from, to } = state.selection;
            return from !== to;
          }}
        >
          {renderItems(bubbleItems)}
        </BubbleMenu>
        {imageBubbleMenu}
        <SlashMenu editor={editor} />
        <UrlInputPopover state={urlPopover} onConfirm={handleUrlConfirm} onCancel={handleUrlCancel} />
        <EditorContent editor={editor} />
      </div>
    );
  }

  return (
    <div className={styles.editorWrapper} style={{ position: 'relative' }}>
      <div className={styles.toolbar}>{renderItems(toolbarItems)}</div>
      <UrlInputPopover state={urlPopover} onConfirm={handleUrlConfirm} onCancel={handleUrlCancel} />
      {imageBubbleMenu}
      <EditorContent editor={editor} />
    </div>
  );
}

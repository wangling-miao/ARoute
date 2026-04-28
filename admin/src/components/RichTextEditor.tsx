import { useCallback, useEffect, useState, useRef } from 'react';
import { useEditor, EditorContent } from '@tiptap/react';
import { BubbleMenu } from '@tiptap/react/menus';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import StarterKit from '@tiptap/starter-kit';
import Underline from '@tiptap/extension-underline';
import Highlight from '@tiptap/extension-highlight';
import Image from '@tiptap/extension-image';
import Link from '@tiptap/extension-link';
import { Table, TableRow, TableCell, TableHeader } from '@tiptap/extension-table';
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
import Placeholder from '@tiptap/extension-placeholder';
import { common, createLowlight } from 'lowlight';
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
} from 'lucide-react';
import styles from './RichTextEditor.module.css';

const lowlight = createLowlight(common);

interface SlashItem {
  title: string;
  description: string;
  icon: React.ReactNode;
  command: (editor: any) => void;
}

interface UrlPopoverState {
  open: boolean;
  mode: 'link' | 'image';
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

  const placeholder = state.mode === 'link' ? 'Paste or type a link…' : 'Paste image URL…';
  const icon = state.mode === 'link' ? <LinkIcon size={14} /> : <ImageIcon size={14} />;

  const submit = () => {
    if (localUrl.trim()) {
      onConfirm(localUrl.trim());
      onCancel();
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') { e.preventDefault(); submit(); }
    if (e.key === 'Escape') onCancel();
  };

  return (
    <div className={styles.urlPopover} style={{ top: state.top, left: state.left }}>
      <span className={styles.urlPopoverIcon}>{icon}</span>
      <input
        ref={inputRef}
        className={styles.urlPopoverInput}
        value={localUrl}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => setLocalUrl(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={placeholder}
        type="url"
      />
      <button type="button" className={styles.urlPopoverBtn} onClick={submit}>
        <ArrowRight size={14} />
      </button>
    </div>
  );
}

const SLASH_ITEMS: SlashItem[] = [
  // Basic blocks
  { title: 'Text', description: 'Plain text block', icon: <Pilcrow size={18} />, command: (ed) => ed.chain().focus().setParagraph().run() },
  { title: 'Heading 1', description: 'Large heading', icon: <Heading1 size={18} />, command: (ed) => ed.chain().focus().toggleHeading({ level: 1 }).run() },
  { title: 'Heading 2', description: 'Medium heading', icon: <Heading2 size={18} />, command: (ed) => ed.chain().focus().toggleHeading({ level: 2 }).run() },
  { title: 'Heading 3', description: 'Small heading', icon: <Heading3 size={18} />, command: (ed) => ed.chain().focus().toggleHeading({ level: 3 }).run() },
  // Lists
  { title: 'Bullet List', description: 'Unordered list', icon: <List size={18} />, command: (ed) => ed.chain().focus().toggleBulletList().run() },
  { title: 'Numbered List', description: 'Ordered list', icon: <ListOrdered size={18} />, command: (ed) => ed.chain().focus().toggleOrderedList().run() },
  // Media & embeds
  { title: 'Image', description: 'Embed an image', icon: <ImageIcon size={18} />, command: () => {} /* handled via popover */ },
  { title: 'Link', description: 'Insert a hyperlink', icon: <LinkIcon size={18} />, command: () => {} /* handled via popover */ },
  { title: 'Table', description: '3×3 table', icon: <TableIcon size={18} />, command: (ed) => ed.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run() },
  // Advanced blocks
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
    const s = stateRef.current;
    const { from } = editor.state.selection;
    const slashPos = from - s.query.length - 1;
    editor.chain().focus().deleteRange({ from: slashPos, to: from }).run();
    item.command(editor);
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
            const textBefore = view.state.doc.textBetween(
              Math.max(0, from - 30), from, '\n'
            );
            const match = textBefore.match(/\/([^\s]*)$/);
            const s = stateRef.current;
            if (match) {
              const q = match[1];
              s.query = q;
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

  // Scroll active item into view
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

function sanitizeUrl(url: string, mode: 'link' | 'image'): string | null {
  if (url.startsWith('/') || url.startsWith('#')) return url;
  try {
    const parsed = new URL(url);
    const allowed = mode === 'image'
      ? ['http:', 'https:', 'data:']
      : ['http:', 'https:', 'mailto:', 'tel:'];
    if (!allowed.includes(parsed.protocol)) return null;
    return url;
  } catch {
    return url;
  }
}

interface RichTextEditorProps {
  value: string;
  onChange: (html: string) => void;
  placeholder?: string;
  notion?: boolean;
}

export default function RichTextEditor({ value, onChange, placeholder, notion }: RichTextEditorProps) {
  const contentClass = notion ? `${styles.editorContent} ${styles.editorNotion}` : styles.editorContent;

  const editor = useEditor({
    extensions: [
      StarterKit.configure({ codeBlock: false }),
      Underline,
      Highlight.configure({ multicolor: false }),
      Image.configure({ inline: false }),
      Link.configure({ openOnClick: false, autolink: true }),
      Table.configure({ resizable: false }),
      TableRow,
      TableCell,
      TableHeader,
      CodeBlockLowlight.configure({ lowlight }),
      Placeholder.configure({ placeholder: placeholder || '' }),
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

  const [urlPopover, setUrlPopover] = useState<UrlPopoverState>({ open: false, mode: 'link', url: '', top: 0, left: 0 });

  const openUrlPopover = useCallback((mode: 'link' | 'image') => {
    if (!editor) return;
    const { from } = editor.state.selection;
    const coords = editor.view.coordsAtPos(from);
    const rect = editor.view.dom.parentElement?.getBoundingClientRect();
    setUrlPopover({
      open: true,
      mode,
      url: '',
      top: coords.bottom - (rect?.top ?? 0) + 8,
      left: coords.left - (rect?.left ?? 0),
    });
  }, [editor]);

  const handleUrlConfirm = useCallback((url: string) => {
    if (!editor || !url) return;
    const sanitized = sanitizeUrl(url.trim(), urlPopover.mode);
    if (!sanitized) return;
    if (urlPopover.mode === 'link') {
      editor.chain().focus().extendMarkRange('link').setLink({ href: sanitized }).run();
    } else {
      editor.chain().focus().setImage({ src: sanitized }).run();
    }
  }, [editor, urlPopover.mode]);

  const handleUrlCancel = useCallback(() => {
    setUrlPopover((s) => ({ ...s, open: false }));
    editor?.commands.focus();
  }, [editor]);

  const insertTable = useCallback(() => {
    if (!editor) return;
    editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run();
  }, [editor]);

  if (!editor) return null;

  type ToolbarBtn = {
    icon: React.ReactNode;
    action: () => void;
    active?: boolean;
    title: string;
  };

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
    { icon: <LinkIcon size={16} />, action: () => openUrlPopover('link'), active: editor.isActive('link'), title: 'Link' },
    { icon: <ImageIcon size={16} />, action: () => openUrlPopover('image'), title: 'Image' },
    { icon: <TableIcon size={16} />, action: insertTable, title: 'Table' },
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
    { icon: <LinkIcon size={14} />, action: () => openUrlPopover('link'), active: editor.isActive('link'), title: 'Link' },
    { icon: <Quote size={14} />, action: () => editor.chain().focus().toggleBlockquote().run(), active: editor.isActive('blockquote'), title: 'Quote' },
    { icon: <List size={14} />, action: () => editor.chain().focus().toggleBulletList().run(), active: editor.isActive('bulletList'), title: 'Bullet List' },
  ];

  const renderItems = (items: (ToolbarBtn | string)[]) =>
    items.map((item, idx) => {
      if (typeof item === 'string') {
        return <div key={`sep-${idx}`} className={styles.toolbarDivider} />;
      }
      return (
        <button
          key={item.title}
          type="button"
          className={`${styles.toolbarBtn} ${item.active ? styles.toolbarBtnActive : ''}`}
          onClick={item.action}
          title={item.title}
          tabIndex={-1}
        >
          {item.icon}
        </button>
      );
    });

  if (notion) {
    return (
      <div className={styles.editorNotionWrapper}>
        <BubbleMenu editor={editor} className={styles.bubbleMenu}>
          {renderItems(bubbleItems)}
        </BubbleMenu>
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
      <EditorContent editor={editor} />
    </div>
  );
}

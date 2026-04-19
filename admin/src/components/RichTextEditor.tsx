import { useCallback, useEffect } from 'react';
import { useEditor, EditorContent } from '@tiptap/react';
import StarterKit from '@tiptap/starter-kit';
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
} from 'lucide-react';
import styles from './RichTextEditor.module.css';

const lowlight = createLowlight(common);

interface RichTextEditorProps {
  value: string;
  onChange: (html: string) => void;
  placeholder?: string;
}

export default function RichTextEditor({ value, onChange, placeholder }: RichTextEditorProps) {
  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        codeBlock: false,
      }),
      Highlight.configure({ multicolor: false }),
      Image.configure({ inline: false }),
      Link.configure({
        openOnClick: false,
        autolink: true,
      }),
      Table.configure({ resizable: false }),
      TableRow,
      TableCell,
      TableHeader,
      CodeBlockLowlight.configure({ lowlight }),
      Placeholder.configure({
        placeholder: placeholder || '',
      }),
    ],
    content: value,
    onUpdate: ({ editor: e }) => {
      onChange(e.getHTML());
    },
    editorProps: {
      attributes: {
        class: styles.editorContent,
      },
    },
  });

  useEffect(() => {
    if (editor && value !== editor.getHTML()) {
      editor.commands.setContent(value);
    }
  }, [value, editor]);

  const addLink = useCallback(() => {
    if (!editor) return;
    const url = window.prompt('URL:');
    if (url) {
      editor.chain().focus().extendMarkRange('link').setLink({ href: url }).run();
    }
  }, [editor]);

  const addImage = useCallback(() => {
    if (!editor) return;
    const url = window.prompt('Image URL:');
    if (url) {
      editor.chain().focus().setImage({ src: url }).run();
    }
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

  type DividerKey = string;

  const toolbarGroups: (ToolbarBtn | DividerKey)[][] = [
    [
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
      { icon: <LinkIcon size={16} />, action: addLink, active: editor.isActive('link'), title: 'Link' },
      { icon: <ImageIcon size={16} />, action: addImage, title: 'Image' },
      { icon: <TableIcon size={16} />, action: insertTable, title: 'Table' },
      { icon: <Minus size={16} />, action: () => editor.chain().focus().setHorizontalRule().run(), title: 'Horizontal Rule' },
      '|',
      { icon: <Undo2 size={16} />, action: () => editor.chain().focus().undo().run(), title: 'Undo' },
      { icon: <Redo2 size={16} />, action: () => editor.chain().focus().redo().run(), title: 'Redo' },
    ],
  ];

  return (
    <div className={styles.editorWrapper}>
      <div className={styles.toolbar}>
        {toolbarGroups.flat().map((item, idx) => {
          if (typeof item === 'string') {
            return <div key={`sep-before-${toolbarGroups.flat().slice(0, idx).filter(i => typeof i !== 'string').length}`} className={styles.toolbarDivider} />;
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
        })}
      </div>
      <EditorContent editor={editor} />
    </div>
  );
}

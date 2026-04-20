"use client";

import { useEditor, EditorContent } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Link from "@tiptap/extension-link";
import Underline from "@tiptap/extension-underline";
import { useEffect } from "react";
import {
  Bold,
  Italic,
  Underline as UnderlineIcon,
  Heading2,
  List,
  ListOrdered,
  LinkIcon,
  Undo,
  Redo,
} from "lucide-react";

interface CampaignEditorProps {
  content: string;
  onChange: (html: string) => void;
}

export function CampaignEditor({ content, onChange }: CampaignEditorProps) {
  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: { levels: [2, 3] },
      }),
      Link.configure({ openOnClick: false }),
      Underline,
    ],
    content,
    onUpdate: ({ editor: e }) => {
      onChange(e.getHTML());
    },
    editorProps: {
      attributes: {
        class:
          "prose prose-sm max-w-none min-h-[200px] px-3 py-2 text-ink-900 focus:outline-none",
      },
    },
  });

  // Sync external content changes (e.g. template selection).
  useEffect(() => {
    if (editor && content !== editor.getHTML()) {
      editor.commands.setContent(content);
    }
    // Only react to content prop changes, not editor HTML.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [content]);

  if (!editor) return null;

  function addLink() {
    if (!editor) return;
    const url = window.prompt("URL");
    if (url) {
      editor.chain().focus().setLink({ href: url }).run();
    }
  }

  return (
    <div className="rounded-md border border-ink-200 bg-background-elevated">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-0.5 border-b border-ink-200 px-2 py-1.5">
        <ToolbarButton
          active={editor.isActive("bold")}
          onClick={() => editor.chain().focus().toggleBold().run()}
          label="Bold"
        >
          <Bold className="size-4" />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("italic")}
          onClick={() => editor.chain().focus().toggleItalic().run()}
          label="Italic"
        >
          <Italic className="size-4" />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("underline")}
          onClick={() => editor.chain().focus().toggleUnderline().run()}
          label="Underline"
        >
          <UnderlineIcon className="size-4" />
        </ToolbarButton>

        <span className="mx-1 h-5 w-px bg-ink-200" aria-hidden />

        <ToolbarButton
          active={editor.isActive("heading", { level: 2 })}
          onClick={() =>
            editor.chain().focus().toggleHeading({ level: 2 }).run()
          }
          label="Heading"
        >
          <Heading2 className="size-4" />
        </ToolbarButton>

        <span className="mx-1 h-5 w-px bg-ink-200" aria-hidden />

        <ToolbarButton
          active={editor.isActive("bulletList")}
          onClick={() => editor.chain().focus().toggleBulletList().run()}
          label="Bullet list"
        >
          <List className="size-4" />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("orderedList")}
          onClick={() => editor.chain().focus().toggleOrderedList().run()}
          label="Numbered list"
        >
          <ListOrdered className="size-4" />
        </ToolbarButton>

        <span className="mx-1 h-5 w-px bg-ink-200" aria-hidden />

        <ToolbarButton
          active={editor.isActive("link")}
          onClick={addLink}
          label="Link"
        >
          <LinkIcon className="size-4" />
        </ToolbarButton>

        <span className="mx-1 h-5 w-px bg-ink-200" aria-hidden />

        <ToolbarButton
          active={false}
          onClick={() => editor.chain().focus().undo().run()}
          label="Undo"
        >
          <Undo className="size-4" />
        </ToolbarButton>
        <ToolbarButton
          active={false}
          onClick={() => editor.chain().focus().redo().run()}
          label="Redo"
        >
          <Redo className="size-4" />
        </ToolbarButton>
      </div>

      {/* Editor */}
      <EditorContent editor={editor} />
    </div>
  );
}

function ToolbarButton({
  active,
  onClick,
  label,
  children,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      aria-pressed={active}
      className={`rounded p-1.5 transition ${
        active
          ? "bg-ink-100 text-ink-900"
          : "text-ink-500 hover:bg-ink-50 hover:text-ink-700"
      }`}
    >
      {children}
    </button>
  );
}

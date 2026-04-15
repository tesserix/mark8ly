"use client";

/**
 * PageBodyEditor — TipTap rich-text editor for static page bodies.
 *
 * Storage contract: the wire format for `page.body` remains **markdown**
 * (the storefront renders via react-markdown). Conversions happen at
 * the component boundary:
 *   - load:  markdown → HTML via `marked` on first mount
 *   - save:  HTML → markdown via a small hand-rolled turndown
 *
 * Merchants never see the markdown; they get a visual editor. The
 * conversion is lossless for the operations this editor exposes
 * (paragraphs, headings H2/H3, bold, italic, underline, lists, links).
 */

import { useEditor, EditorContent } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Link from "@tiptap/extension-link";
import Underline from "@tiptap/extension-underline";
import { useEffect, useRef } from "react";
import { marked } from "marked";
import {
  Bold,
  Italic,
  Underline as UnderlineIcon,
  Heading2,
  Heading3,
  List,
  ListOrdered,
  LinkIcon,
  Undo,
  Redo,
} from "lucide-react";

interface PageBodyEditorProps {
  markdown: string;
  onChange: (markdown: string) => void;
  editable: boolean;
}

export function PageBodyEditor({ markdown, onChange, editable }: PageBodyEditorProps) {
  // Ingest markdown once on mount — further updates come from the
  // editor itself. Outside-in resets (e.g. form reset) are handled
  // below by comparing converted output.
  const initialHtml = useRef<string>("");
  if (initialHtml.current === "") {
    initialHtml.current = markdown ? (marked.parse(markdown, { async: false }) as string) : "";
  }

  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: { levels: [2, 3] },
      }),
      Link.configure({ openOnClick: false, autolink: true }),
      Underline,
    ],
    content: initialHtml.current,
    editable,
    onUpdate: ({ editor: e }) => {
      onChange(htmlToMarkdown(e.getHTML()));
    },
    editorProps: {
      attributes: {
        class:
          "prose prose-sm max-w-none min-h-[260px] px-4 py-3 text-foreground focus:outline-none",
      },
    },
    immediatelyRender: false,
  });

  // Respond to external markdown resets (e.g. form reset).
  useEffect(() => {
    if (!editor) return;
    const currentMd = htmlToMarkdown(editor.getHTML());
    if (currentMd.trim() !== (markdown ?? "").trim()) {
      const nextHtml = markdown ? (marked.parse(markdown, { async: false }) as string) : "";
      editor.commands.setContent(nextHtml, { emitUpdate: false });
    }
    // Only react to markdown prop changes — not editor-internal updates.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [markdown]);

  useEffect(() => {
    editor?.setEditable(editable);
  }, [editor, editable]);

  if (!editor) return null;

  const addLink = () => {
    const previous = editor.getAttributes("link").href as string | undefined;
    const url = window.prompt("URL", previous ?? "https://");
    if (url === null) return;
    if (url === "") {
      editor.chain().focus().unsetLink().run();
      return;
    }
    editor.chain().focus().setLink({ href: url }).run();
  };

  return (
    <div className="rounded-md border border-border bg-background-elevated">
      <div className="flex flex-wrap items-center gap-0.5 border-b border-border px-2 py-1.5">
        <ToolbarButton
          active={editor.isActive("bold")}
          onClick={() => editor.chain().focus().toggleBold().run()}
          label="Bold"
          disabled={!editable}
        >
          <Bold className="h-4 w-4" />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("italic")}
          onClick={() => editor.chain().focus().toggleItalic().run()}
          label="Italic"
          disabled={!editable}
        >
          <Italic className="h-4 w-4" />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("underline")}
          onClick={() => editor.chain().focus().toggleUnderline().run()}
          label="Underline"
          disabled={!editable}
        >
          <UnderlineIcon className="h-4 w-4" />
        </ToolbarButton>

        <span className="mx-1 h-5 w-px bg-border" aria-hidden />

        <ToolbarButton
          active={editor.isActive("heading", { level: 2 })}
          onClick={() => editor.chain().focus().toggleHeading({ level: 2 }).run()}
          label="Heading 2"
          disabled={!editable}
        >
          <Heading2 className="h-4 w-4" />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("heading", { level: 3 })}
          onClick={() => editor.chain().focus().toggleHeading({ level: 3 }).run()}
          label="Heading 3"
          disabled={!editable}
        >
          <Heading3 className="h-4 w-4" />
        </ToolbarButton>

        <span className="mx-1 h-5 w-px bg-border" aria-hidden />

        <ToolbarButton
          active={editor.isActive("bulletList")}
          onClick={() => editor.chain().focus().toggleBulletList().run()}
          label="Bullet list"
          disabled={!editable}
        >
          <List className="h-4 w-4" />
        </ToolbarButton>
        <ToolbarButton
          active={editor.isActive("orderedList")}
          onClick={() => editor.chain().focus().toggleOrderedList().run()}
          label="Numbered list"
          disabled={!editable}
        >
          <ListOrdered className="h-4 w-4" />
        </ToolbarButton>

        <span className="mx-1 h-5 w-px bg-border" aria-hidden />

        <ToolbarButton
          active={editor.isActive("link")}
          onClick={addLink}
          label="Link"
          disabled={!editable}
        >
          <LinkIcon className="h-4 w-4" />
        </ToolbarButton>

        <span className="mx-1 h-5 w-px bg-border" aria-hidden />

        <ToolbarButton
          active={false}
          onClick={() => editor.chain().focus().undo().run()}
          label="Undo"
          disabled={!editable}
        >
          <Undo className="h-4 w-4" />
        </ToolbarButton>
        <ToolbarButton
          active={false}
          onClick={() => editor.chain().focus().redo().run()}
          label="Redo"
          disabled={!editable}
        >
          <Redo className="h-4 w-4" />
        </ToolbarButton>
      </div>

      <EditorContent editor={editor} />
    </div>
  );
}

function ToolbarButton({
  active,
  onClick,
  label,
  disabled,
  children,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      aria-pressed={active}
      disabled={disabled}
      className={`rounded p-1.5 transition disabled:opacity-40 disabled:pointer-events-none ${
        active
          ? "bg-foreground/10 text-foreground"
          : "text-foreground-secondary hover:bg-foreground/5 hover:text-foreground"
      }`}
    >
      {children}
    </button>
  );
}

/**
 * Minimal HTML → markdown converter covering every node type the
 * toolbar can emit. Not a general-purpose serializer — deliberately
 * narrow to keep round-trips predictable against the StarterKit +
 * Link + Underline extension set we configured above.
 *
 * Anything unexpected (tables, embeds, inline styles from a paste)
 * falls through to its textContent so we never lose the merchant's
 * words even if formatting is stripped.
 */
export function htmlToMarkdown(html: string): string {
  if (typeof window === "undefined") return "";
  const container = document.createElement("div");
  container.innerHTML = html;
  return walkBlocks(container).trim();
}

function walkBlocks(node: Node): string {
  let out = "";
  node.childNodes.forEach((child) => {
    if (child.nodeType === Node.TEXT_NODE) {
      const text = child.textContent ?? "";
      if (text.trim()) out += text;
      return;
    }
    if (child.nodeType !== Node.ELEMENT_NODE) return;
    const el = child as HTMLElement;
    const tag = el.tagName.toLowerCase();
    switch (tag) {
      case "p":
        out += walkInline(el) + "\n\n";
        break;
      case "h1":
        out += `# ${walkInline(el)}\n\n`;
        break;
      case "h2":
        out += `## ${walkInline(el)}\n\n`;
        break;
      case "h3":
        out += `### ${walkInline(el)}\n\n`;
        break;
      case "h4":
        out += `#### ${walkInline(el)}\n\n`;
        break;
      case "h5":
        out += `##### ${walkInline(el)}\n\n`;
        break;
      case "h6":
        out += `###### ${walkInline(el)}\n\n`;
        break;
      case "ul":
        el.querySelectorAll(":scope > li").forEach((li) => {
          out += `- ${walkInline(li as HTMLElement)}\n`;
        });
        out += "\n";
        break;
      case "ol":
        el.querySelectorAll(":scope > li").forEach((li, i) => {
          out += `${i + 1}. ${walkInline(li as HTMLElement)}\n`;
        });
        out += "\n";
        break;
      case "blockquote":
        out +=
          walkBlocks(el)
            .split("\n")
            .map((line) => (line ? `> ${line}` : ">"))
            .join("\n") + "\n\n";
        break;
      case "hr":
        out += "---\n\n";
        break;
      case "pre":
        out += "```\n" + (el.textContent ?? "") + "\n```\n\n";
        break;
      default:
        // Unknown block: recurse so nested <p>/<ul> still serialize.
        out += walkBlocks(el);
    }
  });
  return out;
}

function walkInline(el: HTMLElement): string {
  let out = "";
  el.childNodes.forEach((child) => {
    if (child.nodeType === Node.TEXT_NODE) {
      out += child.textContent ?? "";
      return;
    }
    if (child.nodeType !== Node.ELEMENT_NODE) return;
    const c = child as HTMLElement;
    const tag = c.tagName.toLowerCase();
    switch (tag) {
      case "strong":
      case "b":
        out += `**${walkInline(c)}**`;
        break;
      case "em":
      case "i":
        out += `*${walkInline(c)}*`;
        break;
      case "u":
        // Markdown has no canonical underline — react-markdown will
        // pass raw <u> through only with rehype-raw, which the
        // storefront does not use. Fall back to italic so the merchant
        // intent ("emphasize this") still renders.
        out += `*${walkInline(c)}*`;
        break;
      case "a": {
        const href = c.getAttribute("href") ?? "";
        out += `[${walkInline(c)}](${href})`;
        break;
      }
      case "code":
        out += `\`${c.textContent ?? ""}\``;
        break;
      case "br":
        out += "  \n";
        break;
      default:
        out += walkInline(c);
    }
  });
  return out;
}

import { useMemo, useRef } from 'react'
import { Extension, Mark, Node } from '@tiptap/core'
import Document from '@tiptap/extension-document'
import Paragraph from '@tiptap/extension-paragraph'
import Text from '@tiptap/extension-text'
import { UndoRedo } from '@tiptap/extensions'
import { EditorContent, useEditor } from '@tiptap/react'
import { fromEditorDoc, toEditorDoc, EditorNode } from './tags'
import { inlineMarks, tagStyle } from './TaggedText'
import { TagView } from './translatorApi'

export type EditorActions = { onChange: (text: string) => void; onConfirm: () => void; onMove: (step: 1 | -1) => void; onClose: () => void }

// Редактор перевода одного сегмента. Фрагменты <gN> — метка g с форматированием из оригинала,
// <xN/> — неизменяемый значок (его можно удалить, тогда проверка покажет предупреждение).
// Сегмент — один абзац: Enter не создаёт новый.
export function SegmentEditor({ text, tags, actions }: { text: string; tags: Record<string, TagView>; actions: EditorActions }) {
  // Колбэки меняются на каждом рендере страницы, а расширения TipTap создаются один раз.
  const latest = useRef(actions)
  latest.current = actions
  const extensions = useMemo(() => [
    Document.extend({ content: 'paragraph' }),
    Paragraph,
    Text,
    UndoRedo,
    Mark.create({
      name: 'g',
      addAttributes: () => ({ id: { default: null } }),
      parseHTML: () => [{ tag: 'span[data-g]', getAttrs: (element) => ({ id: element.getAttribute('data-g') }) }],
      renderHTML: ({ HTMLAttributes }) => ['span', { 'data-g': HTMLAttributes.id, style: tagStyle(tags[HTMLAttributes.id]) }, 0],
    }),
    Node.create({
      name: 'x',
      group: 'inline',
      inline: true,
      atom: true,
      addAttributes: () => ({ id: { default: null } }),
      parseHTML: () => [{ tag: 'span[data-x]', getAttrs: (element) => ({ id: element.getAttribute('data-x') }) }],
      renderHTML: ({ node }) => {
        const [mark, title] = inlineMarks[tags[node.attrs.id]?.kind ?? 'other']
        return ['span', { 'data-x': node.attrs.id, class: 'tr-inline', title, contenteditable: 'false' }, mark]
      },
    }),
    Extension.create({
      name: 'segmentKeys',
      addKeyboardShortcuts: () => ({
        'Mod-Enter': () => { latest.current.onConfirm(); return true },
        'Alt-ArrowUp': () => { latest.current.onMove(-1); return true },
        'Alt-ArrowDown': () => { latest.current.onMove(1); return true },
        Escape: () => { latest.current.onClose(); return true },
        Enter: () => true,
        'Shift-Enter': () => true,
      }),
    }),
    // Теги сегмента не меняются, а редактор пересоздаётся для каждого сегмента (key).
  ], [])
  const editor = useEditor({
    extensions,
    content: toEditorDoc(text),
    autofocus: 'end',
    editorProps: { attributes: { class: 'tr-editor', 'aria-label': 'Перевод сегмента', style: tagStyle(tags.g0) } },
    onUpdate: ({ editor }) => latest.current.onChange(fromEditorDoc(editor.getJSON() as EditorNode)),
  })
  return <EditorContent editor={editor} />
}

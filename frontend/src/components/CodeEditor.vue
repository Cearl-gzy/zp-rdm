<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { EditorView, basicSetup } from 'codemirror'
import { EditorState, Compartment } from '@codemirror/state'
import { json } from '@codemirror/lang-json'
import { oneDark } from '@codemirror/theme-one-dark'
import { useAppStore } from '../stores/app'

const props = withDefaults(defineProps<{
  modelValue: string
  lang?: 'json' | 'text'
  readonly?: boolean
  placeholder?: string
}>(), { lang: 'text', readonly: false })

const emit = defineEmits<{ 'update:modelValue': [string] }>()

const app = useAppStore()
const hostRef = ref<HTMLElement>()

const langConf = new Compartment()
const themeConf = new Compartment()
const readonlyConf = new Compartment()

let view: EditorView | null = null
let applyingExternal = false

function langExt() {
  return props.lang === 'json' ? [json()] : []
}
function themeExt() {
  return app.dark ? [oneDark] : []
}

onMounted(() => {
  view = new EditorView({
    parent: hostRef.value!,
    state: EditorState.create({
      doc: props.modelValue,
      extensions: [
        basicSetup,
        langConf.of(langExt()),
        themeConf.of(themeExt()),
        readonlyConf.of([EditorState.readOnly.of(props.readonly), EditorView.editable.of(!props.readonly)]),
        EditorView.updateListener.of(u => {
          if (u.docChanged && !applyingExternal) {
            emit('update:modelValue', u.state.doc.toString())
          }
        }),
        EditorView.theme({
          '&': {
            height: '100%',
            fontSize: '12.5px',
            backgroundColor: 'transparent',
          },
          '.cm-scroller': {
            fontFamily: 'var(--mono)',
            lineHeight: '1.65',
          },
          '.cm-content': { padding: '10px 4px' },
          '.cm-gutters': {
            backgroundColor: 'transparent',
            border: 'none',
            color: 'var(--text-3)',
          },
          '&.cm-focused': { outline: 'none' },
          '.cm-activeLine': { backgroundColor: 'rgba(128, 138, 155, 0.08)' },
          '.cm-selectionMatch': { backgroundColor: 'rgba(224, 69, 54, 0.18)' },
        }),
      ],
    }),
  })
})

// 外部值变化（加载完整值 / 切换键）→ 同步到编辑器，避免循环
watch(() => props.modelValue, v => {
  if (!view) return
  const cur = view.state.doc.toString()
  if (v === cur) return
  applyingExternal = true
  view.dispatch({ changes: { from: 0, to: cur.length, insert: v } })
  applyingExternal = false
})

watch(() => props.lang, () => {
  view?.dispatch({ effects: langConf.reconfigure(langExt()) })
})
watch(() => app.dark, () => {
  view?.dispatch({ effects: themeConf.reconfigure(themeExt()) })
})
watch(() => props.readonly, v => {
  view?.dispatch({ effects: readonlyConf.reconfigure([EditorState.readOnly.of(v), EditorView.editable.of(!v)]) })
})

onBeforeUnmount(() => view?.destroy())

function focus() { view?.focus() }
defineExpose({ focus })
</script>

<template>
  <div ref="hostRef" class="code-editor" />
</template>

<style scoped>
.code-editor {
  height: 100%;
  min-height: 0;
  overflow: hidden;
}
.code-editor :deep(.cm-editor) {
  height: 100%;
  border-radius: var(--radius-md);
}
</style>

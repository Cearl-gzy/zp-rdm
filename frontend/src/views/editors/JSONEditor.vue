<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { message } from 'ant-design-vue'
import { SaveOutlined, FormatPainterOutlined, CloudDownloadOutlined, EditOutlined } from '@ant-design/icons-vue'
import { SetJSON, GetFullJSON, SetJSONPath } from '../../../wailsjs/go/service/ValueService'
import { tryFormatJson } from '../../utils/format'
import CodeEditor from '../../components/CodeEditor.vue'

// RedisJSON 文档编辑器（ReJSON-RL 类型）：文本 + 树形双视图，树形支持路径级编辑
const props = defineProps<{
  connId: string
  db: number
  keyName: string
  value: string
  truncated: boolean
  ttl: number
}>()
const emit = defineEmits<{ saved: [] }>()

const text = ref(props.value)
const saving = ref(false)
const loadingFull = ref(false)
const truncatedLocal = ref(props.truncated)
const mode = ref<'text' | 'tree'>('text')

function formatNow() {
  const f = tryFormatJson(text.value)
  if (f) text.value = f
}

watch(() => props.value, v => {
  text.value = v
  truncatedLocal.value = props.truncated
  formatNow()
})
watch(() => props.truncated, v => { truncatedLocal.value = v })
onMounted(formatNow)

function formatJson() {
  const f = tryFormatJson(text.value)
  if (f) { text.value = f; message.success('已格式化') }
  else message.warning('内容不是合法 JSON')
}

async function loadFull() {
  loadingFull.value = true
  try {
    text.value = await GetFullJSON(props.connId, props.db, props.keyName)
    formatNow()
    truncatedLocal.value = false
    message.success('已加载完整内容')
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    loadingFull.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const ttl = props.ttl > 0 ? props.ttl : -1 // -1 保持原 TTL
    await SetJSON(props.connId, props.db, props.keyName, text.value, ttl)
    message.success('已保存')
    emit('saved')
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    saving.value = false
  }
}

// ---------- 树形视图 ----------

interface JsonNode {
  key: string // 唯一 id（JSONPath）
  title: string // 显示名（对象键 / [索引] / 根）
  path: string // JSONPath（$.a.b[0]）
  summary: string // 值摘要
  isLeaf: boolean
  children?: JsonNode[]
}

const TREE_NODE_LIMIT = 3000
let treeNodeCount = 0

// JSONPath 段转义：合法标识符用 .key，否则 ["..."]
function pathSeg(parent: string, seg: string, isIndex: boolean): string {
  if (isIndex) return `${parent}[${seg}]`
  if (/^[A-Za-z_$][A-Za-z0-9_$]*$/.test(seg)) return `${parent}.${seg}`
  return `${parent}[${JSON.stringify(seg)}]`
}

function valueSummary(v: unknown): string {
  if (v === null) return 'null'
  if (typeof v === 'string') return JSON.stringify(v.length > 60 ? v.slice(0, 60) + '…' : v)
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  if (Array.isArray(v)) return `数组 · ${v.length} 项`
  return `对象 · ${Object.keys(v as object).length} 键`
}

function buildNode(name: string, v: unknown, path: string): JsonNode {
  treeNodeCount++
  const isObj = v !== null && typeof v === 'object'
  if (!isObj || treeNodeCount > TREE_NODE_LIMIT) {
    return { key: path, title: name, path, summary: valueSummary(v), isLeaf: !isObj }
  }
  const entries = Array.isArray(v)
    ? v.map((item, i) => buildNode(`[${i}]`, item, pathSeg(path, String(i), true)))
    : Object.entries(v as Record<string, unknown>).map(([k, item]) => buildNode(k, item, pathSeg(path, k, false)))
  return { key: path, title: name, path, summary: valueSummary(v), isLeaf: false, children: entries }
}

const treeData = computed<JsonNode[]>(() => {
  if (mode.value !== 'tree') return []
  let doc: unknown
  try { doc = JSON.parse(text.value) } catch { return [] }
  treeNodeCount = 0
  return [buildNode('$', doc, '$')]
})

// treeNodeCount 非响应式：显式依赖 treeData 保证 treeData 重建后重算
const treeOverflow = computed(() => { void treeData.value; return treeNodeCount > TREE_NODE_LIMIT })
const expandedKeys = ref<string[]>(['$'])

// ---------- 路径级编辑 ----------

const editVisible = ref(false)
const editPath = ref('')
const editText = ref('')
const editSaving = ref(false)

function openNodeEdit(node: JsonNode) {
  editPath.value = node.path
  // 初始值：从当前文档取该路径子树
  try {
    const doc = JSON.parse(text.value)
    const v = resolvePath(doc, node.path)
    editText.value = JSON.stringify(v, null, 2) ?? 'null'
  } catch {
    editText.value = ''
  }
  editVisible.value = true
}

// 由 JSONPath 反解当前文档中的值（编辑初始值用；仅支持本组件生成的规范路径）
function resolvePath(doc: unknown, path: string): unknown {
  if (path === '$') return doc
  const tokens: (string | number)[] = []
  const re = /\.([A-Za-z_$][A-Za-z0-9_$]*)|\[(\d+)\]|\["((?:[^"\\]|\\.)*)"\]/g
  let m: RegExpExecArray | null
  while ((m = re.exec(path)) !== null) {
    if (m[1] !== undefined) tokens.push(m[1])
    else if (m[2] !== undefined) tokens.push(Number(m[2]))
    else tokens.push(JSON.parse(`"${m[3]}"`))
  }
  let cur: unknown = doc
  for (const t of tokens) {
    if (cur === null || typeof cur !== 'object') return undefined
    cur = (cur as Record<string | number, unknown>)[t]
  }
  return cur
}

async function saveNodeEdit() {
  let v = editText.value
  try { JSON.parse(v) } catch { message.warning('内容不是合法 JSON'); return }
  editSaving.value = true
  try {
    await SetJSONPath(props.connId, props.db, props.keyName, editPath.value, v)
    message.success(`已保存 ${editPath.value}`)
    editVisible.value = false
    // 整文档回读，保持文本/树一致
    text.value = await GetFullJSON(props.connId, props.db, props.keyName)
    formatNow()
    truncatedLocal.value = false
    emit('saved')
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    editSaving.value = false
  }
}
</script>

<template>
  <div class="json-editor">
    <div class="je-toolbar">
      <a-tag class="je-tag" color="purple">RedisJSON</a-tag>
      <a-segmented v-model:value="mode" size="small" :options="[{ value: 'text', label: '文本' }, { value: 'tree', label: '树形' }]" />
      <a-button v-if="mode === 'text'" size="small" @click="formatJson">
        <template #icon><FormatPainterOutlined /></template>
        格式化
      </a-button>
      <span class="text-3 je-count">{{ text.length }} 字符</span>

      <template v-if="truncatedLocal">
        <a-tag class="je-tag" color="warning">已截断（预览 1MB）</a-tag>
        <a-button size="small" type="link" :loading="loadingFull" class="load-full-btn" @click="loadFull">
          <template #icon><CloudDownloadOutlined /></template>
          加载完整值
        </a-button>
      </template>

      <a-button v-if="mode === 'text'" type="primary" size="small" style="margin-left: auto" :loading="saving" @click="save">
        <template #icon><SaveOutlined /></template>
        保存
      </a-button>
    </div>

    <div v-show="mode === 'text'" class="je-code-wrap">
      <CodeEditor v-model="text" lang="json" />
    </div>

    <div v-if="mode === 'tree'" class="je-tree-wrap">
      <template v-if="treeData.length">
        <div v-if="treeOverflow" class="je-tree-warn text-3">文档过大，树形仅展示前 {{ TREE_NODE_LIMIT }} 个节点</div>
        <a-tree
          :tree-data="treeData"
          v-model:expanded-keys="expandedKeys"
          :field-names="{ title: 'title', key: 'key', children: 'children' }"
          block-node
          class="je-tree"
        >
          <template #title="node">
            <span class="jt-node">
              <span class="jt-name" :class="{ 'jt-leaf': node.isLeaf }">{{ node.title }}</span>
              <span class="jt-summary mono" :title="node.summary">{{ node.summary }}</span>
              <a-button size="small" type="link" class="jt-edit" @click.stop="openNodeEdit(node)">
                <template #icon><EditOutlined /></template>
              </a-button>
            </span>
          </template>
        </a-tree>
      </template>
      <a-empty v-else description="内容不是合法 JSON，请切换到文本模式修正" style="margin-top: 48px" />
    </div>

    <a-modal
      v-model:open="editVisible"
      :title="`编辑节点`"
      :width="560"
      :confirm-loading="editSaving"
      ok-text="保存"
      cancel-text="取消"
      @ok="saveNodeEdit"
    >
      <div class="jt-path mono">{{ editPath }}</div>
      <div class="jt-editor-wrap">
        <CodeEditor v-model="editText" lang="json" />
      </div>
      <div class="text-3" style="font-size: 11px; margin-top: 6px">
        通过 JSON.SET 路径级写入，仅更新该节点，文档其余部分与 TTL 不受影响
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.json-editor { height: 100%; display: flex; flex-direction: column; gap: 8px; }
.je-toolbar { display: flex; align-items: center; gap: 8px; flex-shrink: 0; flex-wrap: wrap; }
.je-count { font-size: 11px; font-family: var(--mono); }
.je-tag { margin-inline-end: 0 !important; }
.load-full-btn { font-size: 11.5px; height: 22px; padding: 0 6px; }
.je-code-wrap {
  flex: 1; min-height: 0;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  overflow: hidden;
  transition: border-color 0.15s;
}
.je-code-wrap:focus-within { border-color: var(--border-strong); }
.je-tree-wrap {
  flex: 1; min-height: 0; overflow: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: var(--bg-elevated);
  padding: 6px 8px;
}
.je-tree-warn { font-size: 11px; padding: 2px 4px 6px; }
.je-tree :deep(.ant-tree-node-content-wrapper) { min-height: 24px; }
.jt-node { display: inline-flex; align-items: center; gap: 6px; max-width: 100%; }
.jt-name { color: var(--accent); font-size: 12.5px; }
.jt-leaf { color: var(--text-1); }
.jt-summary {
  color: var(--text-3); font-size: 11.5px;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  max-width: 420px; display: inline-block; vertical-align: bottom;
  user-select: text;
}
.jt-edit { visibility: hidden; height: 20px; padding: 0 4px; font-size: 11px; }
.jt-node:hover .jt-edit { visibility: visible; }
.jt-path {
  font-size: 12px; color: var(--accent);
  background: var(--bg-hover); border-radius: 4px;
  padding: 4px 8px; margin-bottom: 8px;
  word-break: break-all; user-select: text;
}
.jt-editor-wrap {
  height: 240px;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  overflow: hidden;
}
</style>

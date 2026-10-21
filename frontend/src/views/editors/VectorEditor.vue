<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, SearchOutlined,
  CopyOutlined, AimOutlined, EyeOutlined, UploadOutlined, DownloadOutlined,
  QuestionCircleOutlined,
} from '@ant-design/icons-vue'
import {
  VectorInfo, VectorSample, VectorSim, VectorEmb, VectorLinks,
  VectorAdd, VectorRemove, VectorGetAttr, VectorSetAttr, VectorImport,
} from '../../../wailsjs/go/service/ValueService'
import { PickDataFile } from '../../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime'
import { useReadonly } from '../../utils/readonly'
import { exportReport, csvCell } from '../../utils/export'
import type { VectorLinkLayer, VectorImportResult } from '../../types'

const props = defineProps<{ connId: string; db: number; keyName: string; total: number }>()

const { t } = useI18n()
const ro = useReadonly(() => props.connId)

// ---------- 概览（VINFO） ----------
const info = ref<Record<string, string>>({})
const count = ref(props.total)

async function loadInfo() {
  try {
    info.value = (await VectorInfo(props.connId, props.db, props.keyName)) as unknown as Record<string, string>
    if (info.value.size) count.value = Number(info.value.size)
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

onMounted(() => { loadInfo(); sample(true) })

const infoPills = computed(() => {
  const i = info.value
  const pills: { label: string; value: string }[] = []
  if (i['vector-dim']) pills.push({ label: t('vector.dim'), value: i['vector-dim'] })
  if (i['quant-type']) pills.push({ label: t('vector.quant'), value: i['quant-type'] })
  pills.push({ label: t('vector.count'), value: String(count.value) })
  if (i['hnsw-m']) pills.push({ label: t('vector.hnswM'), value: i['hnsw-m'] })
  return pills
})

const mode = ref<'sim' | 'members'>('sim')

// ---------- VSIM 查询 ----------
const simMode = ref<'ele' | 'values'>('ele')
const simEle = ref('')
const simVector = ref('')
const simK = ref(10)
const simEf = ref<number | null>(null)
const simFilter = ref('')
const simTruth = ref(false)
const simLoading = ref(false)
const simResults = ref<{ element: string; score: number }[]>([])
const simRan = ref(false)

function parseVector(s: string): number[] | null {
  try {
    const v = JSON.parse(s)
    if (Array.isArray(v) && v.length > 0 && v.every(x => typeof x === 'number')) return v
  } catch { /* fallthrough */ }
  return null
}

// FILTER 表达式客户端预检（括号 / 引号配对；语法细节由服务端校验）
const filterErr = computed(() => {
  const f = simFilter.value.trim()
  if (!f) return ''
  let depth = 0, inStr = false
  for (const ch of f) {
    if (ch === '"') inStr = !inStr
    if (inStr) continue
    if (ch === '(') depth++
    else if (ch === ')' && --depth < 0) return t('vector.filterInvalid')
  }
  return depth !== 0 || inStr ? t('vector.filterInvalid') : ''
})

async function runSim() {
  let values: number[] = []
  if (simMode.value === 'ele') {
    if (!simEle.value.trim()) { message.warning(t('vector.elementName')); return }
  } else {
    const v = parseVector(simVector.value.trim())
    if (!v) { message.warning(t('vector.invalidVector')); return }
    values = v
  }
  if (filterErr.value) { message.warning(filterErr.value); return }
  simLoading.value = true
  try {
    const res = (await VectorSim(
      props.connId, props.db, props.keyName,
      simMode.value === 'ele' ? simEle.value.trim() : '',
      values,
      simK.value > 0 ? simK.value : 10,
      simEf.value && simEf.value > 0 ? simEf.value : 0,
      simFilter.value.trim(),
      simTruth.value,
    )) as unknown as { element: string; score: number }[]
    simResults.value = res || []
    simRan.value = true
  } catch (e: any) {
    message.error(`${t('vector.queryFailed')}: ${String(e?.message ?? e)}`)
  } finally {
    simLoading.value = false
  }
}

function simOfElement(ele: string) {
  mode.value = 'sim'
  simMode.value = 'ele'
  simEle.value = ele
  runSim()
}

// ---------- VSIM 结果导出 ----------
async function exportSim(fmt: 'csv' | 'json') {
  const base = `vsim-${props.keyName.replace(/[^\w.-]+/g, '_')}`
  const content = fmt === 'json'
    ? JSON.stringify(simResults.value, null, 2)
    : ['rank,element,score', ...simResults.value.map((r, i) => `${i + 1},${csvCell(r.element)},${r.score}`)].join('\n')
  await exportReport(`${base}.${fmt}`, content, t('vector.exported'))
}

// ---------- 成员采样浏览 ----------
const members = ref<string[]>([])
const sampleN = ref(50)
const sampling = ref(false)

async function sample(showErr = false) {
  sampling.value = true
  try {
    members.value = ((await VectorSample(props.connId, props.db, props.keyName, sampleN.value)) as unknown as string[]) || []
  } catch (e: any) {
    if (showErr) message.error(String(e?.message ?? e))
  } finally {
    sampling.value = false
  }
}

// ---------- 向量 / 属性 / 邻居图抽屉 ----------
const drawerVisible = ref(false)
const drawerEle = ref('')
const emb = ref<number[]>([])
const attrs = ref('')
const attrsSaving = ref(false)
const links = ref<VectorLinkLayer[]>([])

async function openDetail(ele: string) {
  drawerEle.value = ele
  emb.value = []
  attrs.value = ''
  links.value = []
  drawerVisible.value = true
  try {
    const [v, a, l] = await Promise.all([
      VectorEmb(props.connId, props.db, props.keyName, ele) as unknown as Promise<number[]>,
      VectorGetAttr(props.connId, props.db, props.keyName, ele) as unknown as Promise<string>,
      VectorLinks(props.connId, props.db, props.keyName, ele).catch(() => [] as VectorLinkLayer[]),
    ])
    emb.value = v || []
    attrs.value = a || ''
    links.value = (l as VectorLinkLayer[]) || []
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

// 邻居图布局：中心为当前成员，第 i 层邻居排在第 i 环（L0 最外圈）
const GRAPH_C = 200
const GRAPH_MAX_R = 158

interface GraphNode { ele: string; score: number; layer: number; x: number; y: number }

const graphRings = computed(() =>
  links.value.map((l, i) => ({
    layer: l.layer,
    r: links.value.length > 0 ? GRAPH_MAX_R * (links.value.length - i) / links.value.length : 0,
  })),
)

const graphNodes = computed<GraphNode[]>(() => {
  const out: GraphNode[] = []
  links.value.forEach((l, i) => {
    const r = graphRings.value[i].r
    const n = l.neighbors.length
    l.neighbors.forEach((nb, k) => {
      const a = (2 * Math.PI * k) / n - Math.PI / 2
      out.push({
        ele: nb.element, score: nb.score, layer: l.layer,
        x: GRAPH_C + r * Math.cos(a), y: GRAPH_C + r * Math.sin(a),
      })
    })
  })
  return out
})

function nodeRadius(score: number): number {
  return 4 + Math.max(0, Math.min(1, score)) * 4
}

async function saveAttrs() {
  attrsSaving.value = true
  try {
    await VectorSetAttr(props.connId, props.db, props.keyName, drawerEle.value, attrs.value.trim())
    message.success(t('vector.attrsSaved'))
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    attrsSaving.value = false
  }
}

function copyVector() {
  navigator.clipboard.writeText(JSON.stringify(emb.value))
  message.success(t('vector.copied'))
}

// ---------- 增删 ----------
const addVisible = ref(false)
const addEle = ref('')
const addVector = ref('')
const addAttrs = ref('')
const addQuant = ref('')
const addSaving = ref(false)

function openAdd() {
  addEle.value = ''
  addVector.value = ''
  addAttrs.value = ''
  addQuant.value = ''
  addVisible.value = true
}

async function doAdd() {
  if (!addEle.value.trim()) { message.warning(t('vector.elementName')); return }
  const v = parseVector(addVector.value.trim())
  if (!v) { message.warning(t('vector.invalidVector')); return }
  addSaving.value = true
  try {
    await VectorAdd(props.connId, props.db, props.keyName, addEle.value.trim(), v, addAttrs.value.trim(), addQuant.value)
    message.success(t('vector.added'))
    addVisible.value = false
    loadInfo()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    addSaving.value = false
  }
}

function removeElement(ele: string) {
  Modal.confirm({
    title: t('vector.deleteElement'),
    content: t('vector.deleteConfirm', { ele }),
    okText: t('common.delete'), okButtonProps: { danger: true }, cancelText: t('common.cancel'),
    async onOk() {
      try {
        await VectorRemove(props.connId, props.db, props.keyName, ele)
        message.success(t('vector.deleted'))
        members.value = members.value.filter(m => m !== ele)
        simResults.value = simResults.value.filter(r => r.element !== ele)
        count.value = Math.max(0, count.value - 1)
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

// ---------- 批量导入 ----------
const importVisible = ref(false)
const importPath = ref('')
const importQuant = ref('')
const importing = ref(false)
const importProgress = ref({ done: 0, total: 0, added: 0, failed: 0 })
const importResult = ref<VectorImportResult | null>(null)
const importEvent = computed(() => `vector-import:${props.connId}:${props.db}:${props.keyName}`)

const importPercent = computed(() =>
  importProgress.value.total > 0 ? Math.round((importProgress.value.done / importProgress.value.total) * 100) : 0,
)

function openImport() {
  importPath.value = ''
  importQuant.value = ''
  importResult.value = null
  importProgress.value = { done: 0, total: 0, added: 0, failed: 0 }
  importVisible.value = true
}

async function pickImportFile() {
  try {
    const p = await PickDataFile(t('vector.importTitle'))
    if (p) importPath.value = p
  } catch { /* 取消 */ }
}

async function doImport() {
  if (!importPath.value) { message.warning(t('vector.importPick')); return }
  importing.value = true
  importResult.value = null
  EventsOn(importEvent.value, (p: any) => { importProgress.value = p })
  try {
    const res = (await VectorImport(
      props.connId, props.db, props.keyName, importPath.value, '', importQuant.value,
    )) as unknown as VectorImportResult
    importResult.value = res
    if (res.added > 0) {
      loadInfo()
      message.success(t('vector.importDone', { added: res.added, failed: res.failed }))
    }
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    importing.value = false
    EventsOff(importEvent.value)
  }
}

const simColumns = computed(() => [
  { title: t('vector.colRank'), key: 'rank', width: 48 },
  { title: t('vector.colElement'), dataIndex: 'element', key: 'element' },
  { title: t('vector.colScore'), dataIndex: 'score', key: 'score', width: 180 },
  { title: t('common.actions'), key: 'ops', width: 110 },
])

const memberColumns = computed(() => [
  { title: t('vector.colElement'), dataIndex: 'element', key: 'element' },
  { title: t('common.actions'), key: 'ops', width: 110 },
])

const memberRows = computed(() => members.value.map(m => ({ element: m })))

function scoreColor(s: number): string {
  if (s >= 0.9) return '#3fbf6b'
  if (s >= 0.7) return '#f5a623'
  return 'var(--text-3)'
}
</script>

<template>
  <div class="vector-editor">
    <!-- 概览 -->
    <div class="ve-toolbar">
      <span v-for="p in infoPills" :key="p.label" class="vi-pill">
        <span class="vi-pill-label">{{ p.label }}</span>
        <span class="num">{{ p.value }}</span>
      </span>
      <a-segmented
        v-model:value="mode"
        size="small"
        :options="[
          { value: 'sim', label: t('vector.simQuery') },
          { value: 'members', label: t('vector.members') },
        ]"
        style="margin-left: auto"
      />
      <a-button size="small" @click="loadInfo"><template #icon><ReloadOutlined /></template></a-button>
      <a-button size="small" :disabled="ro" :title="ro ? t('common.readonlyTip') : ''" @click="openImport">
        <template #icon><UploadOutlined /></template>
        {{ t('vector.importBtn') }}
      </a-button>
      <a-button size="small" type="primary" :disabled="ro" :title="ro ? t('common.readonlyTip') : ''" @click="openAdd">
        <template #icon><PlusOutlined /></template>
        {{ t('vector.addElement') }}
      </a-button>
    </div>

    <!-- VSIM 查询 -->
    <template v-if="mode === 'sim'">
      <div class="ve-form">
        <div class="ve-row">
          <a-radio-group v-model:value="simMode" size="small">
            <a-radio-button value="ele">{{ t('vector.queryByEle') }}</a-radio-button>
            <a-radio-button value="values">{{ t('vector.queryByVector') }}</a-radio-button>
          </a-radio-group>
          <a-input
            v-if="simMode === 'ele'"
            v-model:value="simEle"
            size="small"
            :placeholder="t('vector.elementName')"
            class="mono"
            style="flex: 1"
            @press-enter="runSim"
          />
          <a-input-number v-model:value="simK" size="small" :min="1" :max="1000" :placeholder="t('vector.topK')" style="width: 110px" />
          <a-tooltip :title="t('vector.efTip')">
            <a-input-number v-model:value="simEf" size="small" :min="1" :placeholder="t('vector.efLabel')" style="width: 110px" />
          </a-tooltip>
          <a-tooltip :title="t('vector.truthTip')">
            <a-checkbox v-model:checked="simTruth" style="font-size: 12px">{{ t('vector.truthLabel') }}</a-checkbox>
          </a-tooltip>
          <a-button size="small" type="primary" :loading="simLoading" @click="runSim">
            <template #icon><SearchOutlined /></template>
            {{ t('vector.run') }}
          </a-button>
        </div>
        <div v-if="simMode === 'values'" class="ve-row">
          <a-textarea
            v-model:value="simVector"
            :rows="2"
            class="mono"
            :placeholder="t('vector.vectorPlaceholder')"
          />
        </div>
        <div class="ve-row">
          <a-tooltip :title="t('vector.filterTip')" style="flex: 1">
            <a-input
              v-model:value="simFilter"
              size="small"
              class="mono"
              :status="filterErr ? 'error' : ''"
              :placeholder="t('vector.filterLabel') + ' — ' + t('vector.filterPlaceholder')"
              @press-enter="runSim"
            />
          </a-tooltip>
          <a-popover placement="bottomRight" trigger="click">
            <template #title>{{ t('vector.filterHelp') }}</template>
            <template #content>
              <div class="filter-help mono">
                <div>.year &gt;= 2020</div>
                <div>.city == "beijing" and .year &gt; 2020</div>
                <div>(.a &gt; 1 or .b &lt; 2) and .c != "x"</div>
                <div class="text-3" style="margin-top: 6px">{{ t('vector.filterHelpOps') }}</div>
              </div>
            </template>
            <a-button size="small" type="text"><template #icon><QuestionCircleOutlined /></template></a-button>
          </a-popover>
          <a-dropdown v-if="simResults.length" trigger="click">
            <a-button size="small"><template #icon><DownloadOutlined /></template>{{ t('vector.exportResults') }}</a-button>
            <template #overlay>
              <a-menu>
                <a-menu-item key="csv" @click="exportSim('csv')">CSV</a-menu-item>
                <a-menu-item key="json" @click="exportSim('json')">JSON</a-menu-item>
              </a-menu>
            </template>
          </a-dropdown>
        </div>
      </div>

      <div class="ve-table">
        <a-table
          :columns="simColumns"
          :data-source="simResults"
          :pagination="false"
          size="small"
          row-key="element"
          :scroll="{ y: '100%' }"
          :loading="simLoading && !simResults.length"
        >
          <template #bodyCell="{ column, record, index }">
            <template v-if="column.key === 'rank'">
              <span class="text-3 num">{{ index + 1 }}</span>
            </template>
            <template v-else-if="column.key === 'element'">
              <span class="mono cell-value" :title="record.element">{{ record.element }}</span>
            </template>
            <template v-else-if="column.key === 'score'">
              <div class="score-cell">
                <div class="score-bar"><div class="score-fill" :style="{ width: (record.score * 100).toFixed(1) + '%', background: scoreColor(record.score) }" /></div>
                <span class="num" :style="{ color: scoreColor(record.score) }">{{ record.score.toFixed(4) }}</span>
              </div>
            </template>
            <template v-else-if="column.key === 'ops'">
              <a-tooltip :title="t('vector.simOfThis')">
                <a-button size="small" type="text" @click="simOfElement(record.element)"><template #icon><AimOutlined /></template></a-button>
              </a-tooltip>
              <a-tooltip :title="t('vector.viewVector')">
                <a-button size="small" type="text" @click="openDetail(record.element)"><template #icon><EyeOutlined /></template></a-button>
              </a-tooltip>
              <a-button size="small" type="text" danger :disabled="ro" @click="removeElement(record.element)"><template #icon><DeleteOutlined /></template></a-button>
            </template>
          </template>
        </a-table>
        <a-empty
          v-if="simRan && !simResults.length && !simLoading"
          :description="t('vector.noResults')"
          :image-style="{ height: '40px' }"
        />
      </div>
    </template>

    <!-- 成员采样 -->
    <template v-else>
      <div class="ve-form">
        <div class="ve-row">
          <a-tooltip :title="t('vector.sampleTip')">
            <span class="text-3" style="font-size: 12px">{{ t('vector.sample') }}</span>
          </a-tooltip>
          <a-input-number v-model:value="sampleN" size="small" :min="1" :max="500" style="width: 90px" />
          <a-button size="small" :loading="sampling" @click="sample(true)">
            <template #icon><ReloadOutlined /></template>
            {{ t('vector.resample') }}
          </a-button>
          <span class="text-3" style="font-size: 11px; margin-left: auto">{{ t('vector.membersShown', { n: members.length, total: count }) }}</span>
        </div>
      </div>
      <div class="ve-table">
        <a-table
          :columns="memberColumns"
          :data-source="memberRows"
          :pagination="false"
          size="small"
          row-key="element"
          :scroll="{ y: '100%' }"
          :loading="sampling && !members.length"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'element'">
              <span class="mono cell-value" :title="record.element">{{ record.element }}</span>
            </template>
            <template v-else-if="column.key === 'ops'">
              <a-tooltip :title="t('vector.simOfThis')">
                <a-button size="small" type="text" @click="simOfElement(record.element)"><template #icon><AimOutlined /></template></a-button>
              </a-tooltip>
              <a-tooltip :title="t('vector.viewVector')">
                <a-button size="small" type="text" @click="openDetail(record.element)"><template #icon><EyeOutlined /></template></a-button>
              </a-tooltip>
              <a-button size="small" type="text" danger :disabled="ro" @click="removeElement(record.element)"><template #icon><DeleteOutlined /></template></a-button>
            </template>
          </template>
        </a-table>
      </div>
    </template>

    <!-- 添加元素 -->
    <a-modal
      v-model:open="addVisible"
      :title="t('vector.addElement')"
      width="520px"
      :confirm-loading="addSaving"
      :ok-text="t('common.save')" :cancel-text="t('common.cancel')"
      @ok="doAdd"
    >
      <a-form layout="vertical" size="small">
        <a-form-item :label="t('vector.elementName')" required>
          <a-input v-model:value="addEle" class="mono" placeholder="doc:1024" />
        </a-form-item>
        <a-form-item :label="t('vector.vectorJson')" required>
          <a-textarea v-model:value="addVector" :rows="3" class="mono" :placeholder="t('vector.vectorPlaceholder')" />
        </a-form-item>
        <a-form-item :label="t('vector.attrsJson')">
          <a-textarea v-model:value="addAttrs" :rows="2" class="mono" :placeholder="t('vector.attrsPlaceholder')" />
        </a-form-item>
        <a-form-item :label="t('vector.quantOption')">
          <a-select v-model:value="addQuant">
            <a-select-option value="">{{ t('vector.quantDefault') }}</a-select-option>
            <a-select-option value="NOQUANT">NOQUANT (f32)</a-select-option>
            <a-select-option value="Q8">Q8 (int8)</a-select-option>
            <a-select-option value="BIN">BIN (binary)</a-select-option>
          </a-select>
          <div class="text-3" style="font-size: 11px; margin-top: 4px">{{ t('vector.quantTip') }}</div>
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 批量导入 -->
    <a-modal
      v-model:open="importVisible"
      :title="t('vector.importTitle')"
      width="560px"
      :ok-text="t('vector.importStart')" :cancel-text="t('common.cancel')"
      :ok-button-props="{ loading: importing, disabled: !importPath || !!importResult }"
      @ok="doImport"
    >
      <a-form layout="vertical" size="small">
        <a-form-item :label="t('vector.importFile')" required>
          <div style="display: flex; gap: 8px">
            <a-input :value="importPath" readonly class="mono" :placeholder="t('vector.importPick')" style="flex: 1" />
            <a-button @click="pickImportFile">{{ t('vector.importBrowse') }}</a-button>
          </div>
        </a-form-item>
        <a-form-item :label="t('vector.quantOption')">
          <a-select v-model:value="importQuant">
            <a-select-option value="">{{ t('vector.quantDefault') }}</a-select-option>
            <a-select-option value="NOQUANT">NOQUANT (f32)</a-select-option>
            <a-select-option value="Q8">Q8 (int8)</a-select-option>
            <a-select-option value="BIN">BIN (binary)</a-select-option>
          </a-select>
          <div class="text-3" style="font-size: 11px; margin-top: 4px">{{ t('vector.importQuantTip') }}</div>
        </a-form-item>
        <div class="import-fmt text-3 mono">{{ t('vector.importFormats') }}</div>

        <template v-if="importing || importResult">
          <a-progress :percent="importPercent" size="small" :status="importing ? 'active' : (importResult?.failed ? 'exception' : 'success')" />
          <div v-if="importResult" class="import-summary">
            {{ t('vector.importResult', { total: importResult.total, added: importResult.added, failed: importResult.failed }) }}
          </div>
          <div v-if="importResult?.errors.length" class="import-errors">
            <div class="text-3" style="font-size: 11px; margin-bottom: 4px">{{ t('vector.importErrors') }}</div>
            <div v-for="(e, i) in importResult.errors" :key="i" class="mono import-err-line">{{ e }}</div>
            <div v-if="importResult.truncated" class="text-3" style="font-size: 11px">{{ t('vector.importTruncated') }}</div>
          </div>
        </template>
      </a-form>
    </a-modal>

    <!-- 向量 / 属性抽屉 -->
    <a-drawer v-model:open="drawerVisible" :title="drawerEle" width="420" placement="right">
      <div class="drawer-sec">
        <div class="drawer-title">
          {{ t('vector.vectorOf') }}
          <span class="text-3" style="font-weight: 400">({{ emb.length }}d)</span>
          <a-button size="small" type="text" style="margin-left: auto" @click="copyVector">
            <template #icon><CopyOutlined /></template>
          </a-button>
        </div>
        <div class="text-3" style="font-size: 10.5px; margin-bottom: 6px">{{ t('vector.vectorOfTip') }}</div>
        <pre class="emb-view mono">{{ JSON.stringify(emb) }}</pre>
      </div>
      <div class="drawer-sec">
        <div class="drawer-title">{{ t('vector.attrsOf') }}</div>
        <a-textarea v-model:value="attrs" :rows="6" class="mono" :placeholder="t('vector.emptyAttrs')" :disabled="ro" />
        <a-button
          size="small"
          type="primary"
          style="margin-top: 8px"
          :loading="attrsSaving"
          :disabled="ro"
          :title="ro ? t('common.readonlyTip') : ''"
          @click="saveAttrs"
        >
          {{ t('vector.saveAttrs') }}
        </a-button>
      </div>
      <div class="drawer-sec">
        <div class="drawer-title">{{ t('vector.linksTitle') }}</div>
        <div class="text-3" style="font-size: 10.5px; margin-bottom: 6px">{{ t('vector.linksTip') }}</div>
        <a-empty v-if="!links.length" :description="t('vector.linksEmpty')" :image-style="{ height: '40px' }" />
        <svg v-else viewBox="0 0 400 400" class="links-graph">
          <!-- 层环 -->
          <circle
            v-for="ring in graphRings" :key="ring.layer"
            :cx="GRAPH_C" :cy="GRAPH_C" :r="ring.r"
            fill="none" class="ring"
          />
          <text
            v-for="ring in graphRings" :key="'lb' + ring.layer"
            :x="GRAPH_C" :y="GRAPH_C - ring.r - 4"
            class="ring-label" text-anchor="middle"
          >L{{ ring.layer }}</text>
          <!-- 边 -->
          <line
            v-for="n in graphNodes" :key="'e' + n.layer + n.ele"
            :x1="GRAPH_C" :y1="GRAPH_C" :x2="n.x" :y2="n.y"
            class="edge" :style="{ stroke: scoreColor(n.score) }"
          />
          <!-- 中心节点 -->
          <circle :cx="GRAPH_C" :cy="GRAPH_C" r="8" class="center-node" />
          <text :x="GRAPH_C" :y="GRAPH_C + 22" class="node-label center-label" text-anchor="middle">{{ drawerEle }}</text>
          <!-- 邻居节点 -->
          <g
            v-for="n in graphNodes" :key="'n' + n.layer + n.ele"
            class="link-node"
            @click="openDetail(n.ele)"
          >
            <title>{{ n.ele }} · {{ n.score.toFixed(4) }}</title>
            <circle :cx="n.x" :cy="n.y" :r="nodeRadius(n.score)" :style="{ fill: scoreColor(n.score) }" />
            <text :x="n.x" :y="n.y - nodeRadius(n.score) - 3" class="node-label" text-anchor="middle">{{ n.ele }}</text>
          </g>
        </svg>
      </div>
    </a-drawer>
  </div>
</template>

<style scoped>
.vector-editor { height: 100%; display: flex; flex-direction: column; gap: 8px; }
.ve-toolbar { display: flex; gap: 6px; align-items: center; flex-shrink: 0; flex-wrap: wrap; }
.vi-pill {
  display: inline-flex; align-items: center; gap: 5px;
  border: 1px solid var(--border); border-radius: 10px;
  padding: 1px 9px; font-size: 11px;
}
.vi-pill-label { color: var(--text-3); }
.ve-form { display: flex; flex-direction: column; gap: 6px; flex-shrink: 0; }
.ve-row { display: flex; gap: 6px; align-items: center; }
.ve-table { flex: 1; min-height: 0; overflow: hidden; }
.ve-table :deep(.ant-table), .ve-table :deep(.ant-table-container), .ve-table :deep(.ant-table-body) { height: 100%; }
.cell-value { color: var(--text-2); user-select: text; word-break: break-all; white-space: pre-wrap; }
.score-cell { display: flex; align-items: center; gap: 8px; font-size: 11.5px; }
.score-bar { flex: 1; height: 4px; border-radius: 2px; background: var(--bg-hover); overflow: hidden; }
.score-fill { height: 100%; border-radius: 2px; }
.drawer-sec { margin-bottom: 20px; }
.drawer-title { font-size: 12.5px; font-weight: 600; margin-bottom: 6px; display: flex; align-items: center; gap: 6px; }
.emb-view {
  max-height: 220px; overflow: auto;
  background: var(--bg-hover); border-radius: 6px;
  padding: 8px 10px; font-size: 11px;
  white-space: pre-wrap; word-break: break-all;
  user-select: text; margin: 0;
}
.filter-help { font-size: 12px; line-height: 1.9; }
.import-fmt { font-size: 10.5px; line-height: 1.7; margin: 4px 0 12px; }
.import-summary { font-size: 12.5px; font-weight: 600; margin-top: 4px; }
.import-errors {
  margin-top: 8px; max-height: 160px; overflow-y: auto;
  background: var(--bg-hover); border-radius: 6px; padding: 8px 10px;
}
.import-err-line { font-size: 11px; color: #ff8a80; line-height: 1.7; word-break: break-all; }
.links-graph { width: 100%; display: block; }
.ring { stroke: var(--border); stroke-dasharray: 3 4; }
.ring-label { font-size: 9px; fill: var(--text-3); }
.edge { stroke-opacity: 0.35; stroke-width: 1; }
.center-node { fill: var(--accent); }
.center-label { font-weight: 700; fill: var(--text-1); }
.node-label { font-size: 8.5px; fill: var(--text-2); user-select: none; pointer-events: none; }
.link-node { cursor: pointer; }
.link-node:hover circle { stroke: var(--text-1); stroke-width: 1.5; }
</style>

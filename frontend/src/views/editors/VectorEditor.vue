<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { message, Modal } from 'ant-design-vue'
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, SearchOutlined,
  CopyOutlined, AimOutlined, EyeOutlined,
} from '@ant-design/icons-vue'
import {
  VectorInfo, VectorSample, VectorSim, VectorEmb,
  VectorAdd, VectorRemove, VectorGetAttr, VectorSetAttr,
} from '../../../wailsjs/go/service/ValueService'
import { useReadonly } from '../../utils/readonly'

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

async function runSim() {
  let values: number[] = []
  if (simMode.value === 'ele') {
    if (!simEle.value.trim()) { message.warning(t('vector.elementName')); return }
  } else {
    const v = parseVector(simVector.value.trim())
    if (!v) { message.warning(t('vector.invalidVector')); return }
    values = v
  }
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

// ---------- 向量 / 属性抽屉 ----------
const drawerVisible = ref(false)
const drawerEle = ref('')
const emb = ref<number[]>([])
const attrs = ref('')
const attrsSaving = ref(false)

async function openDetail(ele: string) {
  drawerEle.value = ele
  emb.value = []
  attrs.value = ''
  drawerVisible.value = true
  try {
    const [v, a] = await Promise.all([
      VectorEmb(props.connId, props.db, props.keyName, ele) as unknown as Promise<number[]>,
      VectorGetAttr(props.connId, props.db, props.keyName, ele) as unknown as Promise<string>,
    ])
    emb.value = v || []
    attrs.value = a || ''
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
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
              :placeholder="t('vector.filterLabel') + ' — ' + t('vector.filterPlaceholder')"
              @press-enter="runSim"
            />
          </a-tooltip>
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
</style>

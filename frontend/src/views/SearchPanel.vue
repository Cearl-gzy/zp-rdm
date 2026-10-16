<script setup lang="ts">
import { ref, computed, onMounted, watch, h } from 'vue'
import { message, Modal, Checkbox } from 'ant-design-vue'
import { SearchOutlined, LinkOutlined, PlusOutlined, DeleteOutlined } from '@ant-design/icons-vue'
import {
  SearchIndexes, SearchIndexSchema, SearchQuery,
  SearchCreateIndex, SearchDropIndex, SearchAggregate,
} from '../../wailsjs/go/service/ServerService'
import { emitAppEvent, AppEvents } from '../utils/events'

interface SearchField { name: string; type: string; path: string }
interface SearchDoc { key: string; fields: Record<string, string> }
interface SearchResult { total: number; docs: SearchDoc[] }
interface AggRow { [k: string]: string }
interface AggResult { total: number; rows: AggRow[] }

const props = defineProps<{ connId: string; db: number }>()

const mode = ref<'search' | 'agg'>('search')
const indexes = ref<string[]>([])
const index = ref('')
const schema = ref<SearchField[]>([])

// ---- FT.SEARCH ----
const query = ref('')
const searching = ref(false)
const result = ref<SearchResult | null>(null)
const page = ref(1)
const pageSize = 20
const searched = ref(false)

// ---- FT.AGGREGATE ----
const aggQuery = ref('')
const aggGroupBy = ref<string[]>([])
const aggReducers = ref<{ func: string; field: string; alias: string }[]>([])
const agging = ref(false)
const aggResult = ref<AggResult | null>(null)
const aggRan = ref(false)

const aggFuncs = ['COUNT', 'COUNT_DISTINCT', 'SUM', 'AVG', 'MIN', 'MAX']

// ---- 新建索引向导 ----
const createOpen = ref(false)
const creating = ref(false)
const createForm = ref({
  name: '',
  on: 'JSON' as 'JSON' | 'HASH',
  prefixes: '' as string, // 逗号分隔
  fields: [] as { name: string; path: string; type: string; sortable: boolean }[],
})

function openCreate() {
  createForm.value = {
    name: '', on: 'JSON', prefixes: '',
    fields: [{ name: '', path: '$.', type: 'TEXT', sortable: false }],
  }
  createOpen.value = true
}

function addCreateField() {
  createForm.value.fields.push({ name: '', path: createForm.value.on === 'JSON' ? '$.' : '', type: 'TEXT', sortable: false })
}

function removeCreateField(i: number) {
  createForm.value.fields.splice(i, 1)
}

// JSONPath 末段自动填别名（别名为空时）
function autoAlias(f: { name: string; path: string }) {
  if (f.name || !f.path) return
  const seg = f.path.replace(/^\$\.?/, '').split('.').pop() ?? ''
  if (seg && !seg.includes('[')) f.name = seg
}

async function submitCreate() {
  const f = createForm.value
  if (!f.name.trim()) { message.warning('请输入索引名'); return }
  const prefixes = f.prefixes.split(',').map(p => p.trim()).filter(Boolean)
  if (!prefixes.length) { message.warning('至少一个键前缀'); return }
  if (!f.fields.length) { message.warning('至少一个字段'); return }
  creating.value = true
  try {
    await SearchCreateIndex({
      connId: props.connId, db: props.db,
      name: f.name.trim(), on: f.on, prefixes,
      fields: f.fields.map(x => ({ name: x.name.trim(), path: x.path.trim(), type: x.type, sortable: x.sortable })),
    } as any)
    message.success(`索引 ${f.name.trim()} 已创建`)
    createOpen.value = false
    await loadIndexes()
    index.value = f.name.trim()
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    creating.value = false
  }
}

const dropDD = ref(false)

function dropIndex() {
  if (!index.value) return
  const name = index.value
  dropDD.value = false
  Modal.confirm({
    title: `删除索引 ${name}?`,
    content: () => h('label', { style: 'display:flex;align-items:center;gap:6px;cursor:pointer' }, [
      h(Checkbox, {
        checked: dropDD.value,
        'onUpdate:checked': (v: boolean) => { dropDD.value = v },
      }),
      h('span', '连同文档一起删除（DD，不可恢复）'),
    ]),
    okText: '删除',
    okButtonProps: { danger: true },
    cancelText: '取消',
    onOk: async () => {
      try {
        await SearchDropIndex(props.connId, props.db, name, dropDD.value)
        message.success(`索引 ${name} 已删除`)
        result.value = null; aggResult.value = null; searched.value = false; aggRan.value = false
        await loadIndexes()
      } catch (e: any) {
        message.error(String(e?.message ?? e))
      }
    },
  })
}

async function loadIndexes() {
  try {
    indexes.value = ((await SearchIndexes(props.connId, props.db)) as unknown as string[]) || []
    if (indexes.value.length && !indexes.value.includes(index.value)) {
      index.value = indexes.value[0]
    } else if (!indexes.value.length) {
      index.value = ''
    }
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

async function loadSchema() {
  schema.value = []
  if (!index.value) return
  try {
    schema.value = ((await SearchIndexSchema(props.connId, props.db, index.value)) as unknown as SearchField[]) || []
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}

watch(index, () => {
  loadSchema()
  result.value = null; searched.value = false; page.value = 1
  aggResult.value = null; aggRan.value = false
})

async function search(toPage = 1) {
  if (!index.value) { message.warning('请选择索引'); return }
  if (!query.value.trim()) { message.warning('请输入查询语句'); return }
  searching.value = true
  try {
    page.value = toPage
    result.value = (await SearchQuery(
      props.connId, props.db, index.value, query.value.trim(),
      (toPage - 1) * pageSize, pageSize,
    )) as unknown as SearchResult
    searched.value = true
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    searching.value = false
  }
}

function addReducer() {
  aggReducers.value.push({ func: 'COUNT', field: '', alias: '' })
}

function removeReducer(i: number) {
  aggReducers.value.splice(i, 1)
}

async function runAgg() {
  if (!index.value) { message.warning('请选择索引'); return }
  agging.value = true
  try {
    aggResult.value = (await SearchAggregate({
      connId: props.connId, db: props.db,
      index: index.value, query: aggQuery.value.trim(),
      groupBy: aggGroupBy.value,
      reducers: aggReducers.value.map(r => ({ func: r.func, field: r.field, alias: r.alias.trim() })),
      limit: 200,
    } as any)) as unknown as AggResult
    aggRan.value = true
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  } finally {
    agging.value = false
  }
}

// 聚合结果动态列
const aggColumns = computed(() => {
  const cols: string[] = []
  for (const row of aggResult.value?.rows ?? []) {
    for (const k of Object.keys(row)) {
      if (!cols.includes(k)) cols.push(k)
    }
  }
  return cols.map(k => ({ title: k, key: k, ellipsis: true }))
})

// 内容摘要：ON JSON 索引返回 $ 完整文档；ON HASH 为逐字段
function docSummary(doc: SearchDoc): string {
  const dollar = doc.fields['$']
  if (dollar) {
    try { return JSON.stringify(JSON.parse(dollar)) } catch { return dollar }
  }
  return Object.entries(doc.fields).map(([k, v]) => `${k}=${v}`).join('  ')
}

function openKey(key: string) {
  emitAppEvent(AppEvents.OpenKey, { connId: props.connId, key })
}

const fieldTypeColor: Record<string, string> = {
  TEXT: 'blue', TAG: 'green', NUMERIC: 'orange', GEO: 'purple', VECTOR: 'magenta',
}

const columns = [
  { title: '键', key: 'key', width: 220 },
  { title: '内容', key: 'content' },
  { title: '', key: 'action', width: 70 },
]

const total = computed(() => result.value?.total ?? 0)
const schemaFieldOptions = computed(() => schema.value.map(f => ({ value: f.name, label: `${f.name} (${f.type})` })))

onMounted(loadIndexes)
</script>

<template>
  <div class="search-panel">
    <div class="sp-toolbar">
      <a-segmented v-model:value="mode" size="small" :options="[{ value: 'search', label: '搜索' }, { value: 'agg', label: '聚合' }]" />
      <a-select
        v-model:value="index"
        size="small"
        style="width: 170px"
        placeholder="选择索引"
        :options="indexes.map(i => ({ value: i, label: i }))"
        :not-found-content="null"
      />
      <a-tooltip title="新建索引">
        <a-button size="small" @click="openCreate">
          <template #icon><PlusOutlined /></template>
        </a-button>
      </a-tooltip>
      <a-tooltip title="删除当前索引">
        <a-button size="small" danger :disabled="!index" @click="dropIndex">
          <template #icon><DeleteOutlined /></template>
        </a-button>
      </a-tooltip>

      <template v-if="mode === 'search'">
        <a-input
          v-model:value="query"
          size="small"
          class="sp-query mono"
          placeholder='查询语法：* | hello | @name:alice | @age:[18 30] | @city:{北京}'
          allow-clear
          @press-enter="search(1)"
        />
        <a-button size="small" type="primary" :loading="searching" @click="search(1)">
          <template #icon><SearchOutlined /></template>
          搜索
        </a-button>
      </template>
      <template v-else>
        <a-input
          v-model:value="aggQuery"
          size="small"
          class="sp-query mono"
          placeholder="过滤查询（空 = *），如 @region:{north}"
          allow-clear
          @press-enter="runAgg"
        />
        <a-button size="small" type="primary" :loading="agging" @click="runAgg">
          <template #icon><SearchOutlined /></template>
          聚合
        </a-button>
      </template>
    </div>

    <div v-if="schema.length" class="sp-schema">
      <span class="text-3" style="font-size: 11px">字段：</span>
      <a-tag v-for="f in schema" :key="f.name" :color="fieldTypeColor[f.type] ?? 'default'" class="sp-field-tag">
        <span class="mono">{{ f.name }}</span>
        <span class="sp-field-type">{{ f.type }}</span>
        <span v-if="f.path && f.path !== f.name" class="text-3" style="font-size: 10.5px">{{ f.path }}</span>
      </a-tag>
    </div>

    <!-- 聚合构造器 -->
    <div v-if="mode === 'agg' && index" class="sp-agg-builder">
      <div class="sp-agg-row">
        <span class="sp-agg-label">GROUP BY</span>
        <a-select
          v-model:value="aggGroupBy"
          mode="multiple"
          size="small"
          class="sp-agg-fields"
          placeholder="分组字段（可不选 = 全量聚合）"
          :options="schemaFieldOptions"
          :max-tag-count="3"
        />
      </div>
      <div v-for="(r, i) in aggReducers" :key="i" class="sp-agg-row">
        <span class="sp-agg-label">REDUCE</span>
        <a-select v-model:value="r.func" size="small" style="width: 150px"
          :options="aggFuncs.map(f => ({ value: f, label: f }))" />
        <a-select v-if="r.func !== 'COUNT'" v-model:value="r.field" size="small" style="width: 160px"
          placeholder="字段" :options="schemaFieldOptions" allow-clear />
        <a-input v-model:value="r.alias" size="small" style="width: 130px" placeholder="别名（可空）" class="mono" allow-clear />
        <a-button size="small" type="text" danger @click="removeReducer(i)">
          <template #icon><DeleteOutlined /></template>
        </a-button>
      </div>
      <div class="sp-agg-row">
        <a-button size="small" type="dashed" @click="addReducer">
          <template #icon><PlusOutlined /></template>
          添加聚合器
        </a-button>
      </div>
    </div>

    <div class="sp-body">
      <a-empty v-if="!indexes.length" style="margin-top: 60px">
        <template #description>
          当前 DB 没有 RediSearch 索引，
          <a-button type="link" size="small" style="padding: 0" @click="openCreate">新建一个</a-button>
        </template>
      </a-empty>

      <!-- 搜索结果 -->
      <template v-else-if="mode === 'search'">
        <a-empty v-if="searched && !total" description="无匹配结果" style="margin-top: 60px" />
        <a-empty v-else-if="!searched" description="输入查询语句开始搜索，支持 RediSearch 查询语法" style="margin-top: 60px" />
        <template v-else>
          <a-table
            :columns="columns"
            :data-source="result?.docs ?? []"
            :pagination="false"
            size="small"
            row-key="key"
            :loading="searching"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'key'">
                <span class="mono sp-key" :title="record.key">{{ record.key }}</span>
              </template>
              <template v-else-if="column.key === 'content'">
                <span class="mono sp-doc" :title="docSummary(record)">{{ docSummary(record) }}</span>
              </template>
              <template v-else-if="column.key === 'action'">
                <a-button size="small" type="link" @click="openKey(record.key)">
                  <template #icon><LinkOutlined /></template>
                </a-button>
              </template>
            </template>
          </a-table>
          <div class="sp-pager">
            <span class="text-3" style="font-size: 11.5px">共 {{ total }} 条</span>
            <a-pagination
              :current="page"
              :page-size="pageSize"
              :total="total"
              size="small"
              :show-less-items="true"
              @change="(p: number) => search(p)"
            />
          </div>
        </template>
      </template>

      <!-- 聚合结果 -->
      <template v-else>
        <a-empty v-if="aggRan && !(aggResult?.rows.length)" description="无聚合结果" style="margin-top: 60px" />
        <a-empty v-else-if="!aggRan" description="选择分组字段与聚合器，或直接全量聚合" style="margin-top: 60px" />
        <template v-else>
          <a-table
            :columns="aggColumns"
            :data-source="aggResult?.rows ?? []"
            :pagination="false"
            size="small"
            :row-key="(r: AggRow) => JSON.stringify(r)"
            :loading="agging"
          >
            <template #bodyCell="{ column, record }">
              <span class="mono sp-doc">{{ record[column.key] }}</span>
            </template>
          </a-table>
          <div class="sp-pager">
            <span class="text-3" style="font-size: 11.5px">{{ aggResult?.rows.length ?? 0 }} 行（上限 200）</span>
          </div>
        </template>
      </template>
    </div>

    <!-- 新建索引向导 -->
    <a-modal
      v-model:open="createOpen"
      title="新建索引"
      width="640px"
      :confirm-loading="creating"
      ok-text="创建"
      cancel-text="取消"
      @ok="submitCreate"
    >
      <a-form layout="vertical" size="small">
        <div class="sp-form-row">
          <a-form-item label="索引名" class="sp-form-grow">
            <a-input v-model:value="createForm.name" class="mono" placeholder="idx:users" />
          </a-form-item>
          <a-form-item label="数据类型" style="width: 160px">
            <a-radio-group v-model:value="createForm.on" option-type="button" button-style="solid"
              :options="[{ value: 'JSON', label: 'JSON' }, { value: 'HASH', label: 'HASH' }]" />
          </a-form-item>
        </div>
        <a-form-item label="键前缀（逗号分隔，命中前缀的键才会被索引）">
          <a-input v-model:value="createForm.prefixes" class="mono" placeholder="user:, profile:" />
        </a-form-item>
        <a-form-item label="Schema 字段">
          <div v-for="(f, i) in createForm.fields" :key="i" class="sp-field-row">
            <a-input
              v-model:value="f.path"
              class="mono sp-field-path"
              :placeholder="createForm.on === 'JSON' ? '$.name' : '字段名（可空=别名）'"
              @blur="autoAlias(f)"
            />
            <a-input v-model:value="f.name" class="mono" style="width: 120px" placeholder="别名" />
            <a-select v-model:value="f.type" style="width: 110px"
              :options="['TEXT', 'TAG', 'NUMERIC', 'GEO'].map(t => ({ value: t, label: t }))" />
            <a-checkbox v-model:checked="f.sortable">Sortable</a-checkbox>
            <a-button size="small" type="text" danger :disabled="createForm.fields.length <= 1" @click="removeCreateField(i)">
              <template #icon><DeleteOutlined /></template>
            </a-button>
          </div>
          <a-button size="small" type="dashed" block style="margin-top: 6px" @click="addCreateField">
            <template #icon><PlusOutlined /></template>
            添加字段
          </a-button>
          <div class="text-3" style="font-size: 11px; margin-top: 6px">
            复杂场景（VECTOR / SORTABLE UNF / 停用词等）请用 CLI 执行原生命令 FT.CREATE
          </div>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<style scoped>
.search-panel { height: 100%; display: flex; flex-direction: column; }
.sp-toolbar {
  display: flex; align-items: center; gap: 8px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.sp-query { flex: 1; }
.sp-schema {
  display: flex; align-items: center; gap: 6px; flex-wrap: wrap;
  padding: 6px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}
.sp-field-tag { display: inline-flex; align-items: center; gap: 4px; margin-inline-end: 0; }
.sp-field-type { font-size: 10px; opacity: 0.75; }
.sp-agg-builder {
  padding: 8px 16px;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
  display: flex; flex-direction: column; gap: 6px;
}
.sp-agg-row { display: flex; align-items: center; gap: 8px; }
.sp-agg-label { font-size: 11px; color: var(--text-3); width: 64px; flex-shrink: 0; font-family: monospace; }
.sp-agg-fields { flex: 1; }
.sp-body { flex: 1; overflow: auto; padding: 10px 16px; }
.sp-key { color: var(--accent); user-select: text; }
.sp-doc { color: var(--text-2); user-select: text; word-break: break-all; white-space: pre-wrap; font-size: 11.5px; }
.sp-pager {
  display: flex; align-items: center; justify-content: space-between;
  padding: 10px 0 2px;
}
.sp-form-row { display: flex; gap: 12px; }
.sp-form-grow { flex: 1; }
.sp-field-row { display: flex; align-items: center; gap: 6px; margin-bottom: 6px; }
.sp-field-path { flex: 1; }
</style>

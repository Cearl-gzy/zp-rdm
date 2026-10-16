<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { message } from 'ant-design-vue'
import { SearchOutlined, LinkOutlined } from '@ant-design/icons-vue'
import { SearchIndexes, SearchIndexSchema, SearchQuery } from '../../wailsjs/go/service/ServerService'
import { emitAppEvent, AppEvents } from '../utils/events'

interface SearchField { name: string; type: string; path: string }
interface SearchDoc { key: string; fields: Record<string, string> }
interface SearchResult { total: number; docs: SearchDoc[] }

const props = defineProps<{ connId: string; db: number }>()

const indexes = ref<string[]>([])
const index = ref('')
const schema = ref<SearchField[]>([])
const query = ref('')
const searching = ref(false)
const result = ref<SearchResult | null>(null)
const page = ref(1)
const pageSize = 20
const searched = ref(false) // 是否执行过搜索（区分初始空态与无结果）

async function loadIndexes() {
  try {
    indexes.value = ((await SearchIndexes(props.connId, props.db)) as unknown as string[]) || []
    if (indexes.value.length && !indexes.value.includes(index.value)) {
      index.value = indexes.value[0]
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

watch(index, () => { loadSchema(); result.value = null; searched.value = false; page.value = 1 })

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

onMounted(loadIndexes)
</script>

<template>
  <div class="search-panel">
    <div class="sp-toolbar">
      <a-select
        v-model:value="index"
        size="small"
        style="width: 180px"
        placeholder="选择索引"
        :options="indexes.map(i => ({ value: i, label: i }))"
        :not-found-content="null"
      />
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
    </div>

    <div v-if="schema.length" class="sp-schema">
      <span class="text-3" style="font-size: 11px">字段：</span>
      <a-tag v-for="f in schema" :key="f.name" :color="fieldTypeColor[f.type] ?? 'default'" class="sp-field-tag">
        <span class="mono">{{ f.name }}</span>
        <span class="sp-field-type">{{ f.type }}</span>
        <span v-if="f.path && f.path !== f.name" class="text-3" style="font-size: 10.5px">{{ f.path }}</span>
      </a-tag>
    </div>

    <div class="sp-body">
      <a-empty v-if="!indexes.length" description="当前 DB 没有 RediSearch 索引" style="margin-top: 60px" />
      <a-empty v-else-if="searched && !total" description="无匹配结果" style="margin-top: 60px" />
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
    </div>
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
.sp-body { flex: 1; overflow: auto; padding: 10px 16px; }
.sp-key { color: var(--accent); user-select: text; }
.sp-doc { color: var(--text-2); user-select: text; word-break: break-all; white-space: pre-wrap; font-size: 11.5px; }
.sp-pager {
  display: flex; align-items: center; justify-content: space-between;
  padding: 10px 0 2px;
}
</style>

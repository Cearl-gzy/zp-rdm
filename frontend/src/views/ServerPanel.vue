<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ServerConfig from './ServerConfig.vue'
import AclPanel from './AclPanel.vue'
import ReplicationPanel from './ReplicationPanel.vue'

// 服务器管理面板：复制拓扑 + 运行配置（CONFIG）+ ACL 用户管理（集群模式不展示此视图）
const props = defineProps<{ connId: string; db: number }>()
const { t } = useI18n()

const tab = ref<'replication' | 'config' | 'acl'>('replication')
</script>

<template>
  <div class="server-panel">
    <a-tabs v-model:activeKey="tab" size="small" class="sp-tabs">
      <a-tab-pane key="replication" :tab="t('serverPanel.tabReplication')">
        <ReplicationPanel v-if="tab === 'replication'" :conn-id="props.connId" :db="props.db" />
      </a-tab-pane>
      <a-tab-pane key="config" :tab="t('serverPanel.tabConfig')">
        <ServerConfig v-if="tab === 'config'" :conn-id="props.connId" :db="props.db" />
      </a-tab-pane>
      <a-tab-pane key="acl" :tab="t('serverPanel.tabAcl')">
        <AclPanel v-if="tab === 'acl'" :conn-id="props.connId" :db="props.db" />
      </a-tab-pane>
    </a-tabs>
  </div>
</template>

<style scoped>
.server-panel { height: 100%; display: flex; flex-direction: column; }
.sp-tabs { height: 100%; display: flex; flex-direction: column; padding: 0 16px; }
.sp-tabs :deep(.ant-tabs-content-holder) { flex: 1; min-height: 0; }
.sp-tabs :deep(.ant-tabs-content),
.sp-tabs :deep(.ant-tabs-tabpane) { height: 100%; }
</style>

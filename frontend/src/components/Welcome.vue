<script setup lang="ts">
import { useConnectionStore } from '../stores/connection'
import { message } from 'ant-design-vue'
import {
  DatabaseOutlined, ThunderboltOutlined, SafetyOutlined, CodeOutlined, ArrowRightOutlined,
} from '@ant-design/icons-vue'

const conn = useConnectionStore()

const features = [
  { icon: DatabaseOutlined, title: '全数据类型', desc: 'String / Hash / List / Set / ZSet / Stream 可视化编辑' },
  { icon: ThunderboltOutlined, title: '实时监控', desc: 'INFO 仪表盘、OPS 趋势、慢日志、客户端管理' },
  { icon: SafetyOutlined, title: '安全连接', desc: 'ACL 认证、TLS 加密、SSH 隧道' },
  { icon: CodeOutlined, title: 'CLI 终端', desc: '任意 Redis 命令，历史记录与格式化输出' },
]

async function quickOpen(id: string) {
  try {
    await conn.open(id)
  } catch (e: any) {
    message.error(String(e?.message ?? e))
  }
}
</script>

<template>
  <div class="welcome">
    <div class="welcome-inner">
      <div class="hero">
        <div class="hero-logo">ZP</div>
        <div class="hero-text">
          <h1 class="hero-title">ZP Redis Manager</h1>
          <p class="hero-sub">轻量、精美、功能完善的 Redis 桌面管理工具</p>
        </div>
      </div>

      <div v-if="conn.profiles.length" class="quick-block">
        <div class="block-label">快速连接</div>
        <div class="quick-list">
          <div
            v-for="p in conn.profiles.slice(0, 5)"
            :key="p.id"
            class="quick-item"
            @click="quickOpen(p.id)"
          >
            <span class="q-color" :style="{ background: p.color || '#e04536' }" />
            <div class="q-meta">
              <div class="q-name">{{ p.name }}</div>
              <div class="q-addr num">{{ p.host }}:{{ p.port }}</div>
            </div>
            <ArrowRightOutlined class="q-arrow" />
          </div>
        </div>
      </div>
      <div v-else class="quick-block">
        <div class="block-label">开始使用</div>
        <div class="start-hint">在左侧点击「新建」创建你的第一个 Redis 连接</div>
      </div>

      <div class="feature-grid">
        <div v-for="f in features" :key="f.title" class="feature">
          <div class="f-icon"><component :is="f.icon" /></div>
          <div class="f-body">
            <div class="f-title">{{ f.title }}</div>
            <div class="f-desc">{{ f.desc }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.welcome {
  height: 100%;
  overflow-y: auto;
  display: flex;
  justify-content: center;
  background: var(--bg-base);
}
.welcome-inner {
  width: 100%;
  max-width: 720px;
  padding: 72px 40px 48px;
  display: flex;
  flex-direction: column;
  gap: 40px;
}

.hero {
  display: flex;
  align-items: center;
  gap: 18px;
}
.hero-logo {
  width: 56px; height: 56px;
  border-radius: 15px;
  display: flex; align-items: center; justify-content: center;
  font-size: 20px; font-weight: 800; letter-spacing: 0.5px;
  color: #fff;
  background: linear-gradient(140deg, #f0583f 0%, #b3251a 100%);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.22), 0 6px 20px rgba(224, 69, 54, 0.28);
  flex-shrink: 0;
}
.hero-title {
  margin: 0;
  font-size: 26px; font-weight: 750;
  letter-spacing: -0.3px;
  line-height: 1.25;
}
.hero-sub { color: var(--text-2); margin: 5px 0 0; font-size: 13px; }

.block-label {
  font-size: 11px; font-weight: 600;
  color: var(--text-3);
  letter-spacing: 1px;
  text-transform: uppercase;
  margin-bottom: 10px;
}

.quick-list {
  display: flex;
  flex-direction: column;
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  overflow: hidden;
}
.quick-item {
  display: flex; align-items: center; gap: 12px;
  padding: 12px 16px;
  cursor: pointer;
  transition: background 0.12s var(--ease);
  border-bottom: 1px solid var(--border);
}
.quick-item:last-child { border-bottom: none; }
.quick-item:hover { background: var(--bg-hover); }
.quick-item:active { transform: translateY(1px); }
.q-color { width: 3px; height: 26px; border-radius: 2px; flex-shrink: 0; }
.q-meta { flex: 1; min-width: 0; }
.q-name { font-weight: 600; font-size: 13px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.q-addr { font-size: 11px; color: var(--text-3); margin-top: 1px; }
.q-arrow {
  color: var(--text-3);
  font-size: 12px;
  opacity: 0;
  transform: translateX(-4px);
  transition: opacity 0.15s, transform 0.15s var(--ease);
}
.quick-item:hover .q-arrow { opacity: 1; transform: translateX(0); }

.start-hint {
  color: var(--text-3);
  font-size: 12.5px;
  padding: 22px;
  border: 1px dashed var(--border-strong);
  border-radius: var(--radius-lg);
  text-align: center;
}

.feature-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
@media (max-width: 640px) { .feature-grid { grid-template-columns: 1fr; } }
.feature {
  display: flex;
  gap: 13px;
  padding: 15px 16px;
  background: var(--bg-panel);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  transition: border-color 0.18s var(--ease), transform 0.18s var(--ease), box-shadow 0.18s var(--ease);
}
.feature:hover {
  border-color: var(--border-strong);
  transform: translateY(-2px);
  box-shadow: var(--shadow-card);
}
.feature:hover .f-icon { transform: scale(1.06); }
.f-icon { transition: transform 0.18s var(--ease); }
.f-icon {
  width: 32px; height: 32px;
  border-radius: 8px;
  display: flex; align-items: center; justify-content: center;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 15px;
  flex-shrink: 0;
}
.f-title { font-weight: 600; font-size: 12.5px; margin-bottom: 3px; }
.f-desc { font-size: 11.5px; color: var(--text-3); line-height: 1.55; }
</style>

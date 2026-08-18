<template>
  <div>
    <el-card shadow="never">
      <div class="welcome">
        <div>
          <h2>{{ greeting }}，{{ auth.user?.name || auth.user?.username }}</h2>
          <p class="sub">
            {{ auth.isAdmin
              ? '您可以管理组织与账号、创建会议、展示汇报并生成纪要。'
              : '您可以为所属组织录入会议汇报事项，并及时补充更新。' }}
          </p>
        </div>
        <el-button v-if="auth.isAdmin" type="primary" size="large" @click="$router.push('/meetings')">
          创建 / 查看会议
        </el-button>
      </div>
    </el-card>

    <el-row :gutter="16" class="stats">
      <el-col v-for="s in statCards" :key="s.status" :span="4">
        <el-card shadow="hover" class="stat-card" @click="goByStatus(s.status)">
          <div class="stat">
            <div class="num">{{ s.count }}</div>
            <div class="label">{{ s.label }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" class="recent">
      <template #header>最近会议</template>
      <el-table :data="recentMeetings" @row-click="goDetail" style="cursor: pointer">
        <el-table-column prop="title" label="会议名称" min-width="220" />
        <el-table-column label="会议时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.meeting_time) }}</template>
        </el-table-column>
        <el-table-column label="参会组织" min-width="180">
          <template #default="{ row }">
            {{ (row.orgs || []).map((o) => o.org?.name).filter(Boolean).join('、') || '—' }}
          </template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)">{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!recentMeetings.length" description="暂无会议" />
    </el-card>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { meetingApi } from '../api'

const auth = useAuthStore()
const router = useRouter()
const meetings = ref([])

const greeting = computed(() => {
  const h = new Date().getHours()
  if (h < 6) return '夜深了'
  if (h < 12) return '上午好'
  if (h < 14) return '中午好'
  if (h < 18) return '下午好'
  return '晚上好'
})

const stats = computed(() => {
  const s = { total: meetings.value.length, draft: 0, ongoing: 0, finished: 0, archived: 0 }
  for (const m of meetings.value) {
    if (s[m.status] !== undefined) s[m.status]++
  }
  return s
})

const recentMeetings = computed(() =>
  [...meetings.value].sort((a, b) => new Date(b.meeting_time) - new Date(a.meeting_time)).slice(0, 8),
)

function fmtTime(t) {
  if (!t) return '—'
  return t.slice(0, 16).replace('T', ' ')
}
function statusText(s) {
  return { draft: '筹备中', ongoing: '进行中', finished: '已结束', archived: '已归档' }[s] || s
}
function statusType(s) {
  return { draft: 'info', ongoing: 'warning', finished: 'success', archived: 'danger' }[s] || 'info'
}
function goDetail(row) {
  router.push(`/meetings/${row.id}`)
}

// 首页统计卡片：点击进入会议管理并按状态过滤
const statCards = computed(() => [
  { status: '', count: stats.value.total, label: '全部会议' },
  { status: 'draft', count: stats.value.draft, label: '筹备中（可录入）' },
  { status: 'ongoing', count: stats.value.ongoing, label: '进行中' },
  { status: 'finished', count: stats.value.finished, label: '已结束' },
  { status: 'archived', count: stats.value.archived, label: '已归档' },
])
function goByStatus(status) {
  const query = status ? { status } : {}
  router.push({ path: '/meetings', query })
}

onMounted(async () => {
  meetings.value = await meetingApi.list({ limit: 0 })
})
</script>

<style scoped>
.welcome {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.welcome h2 {
  margin: 0 0 6px;
}
.sub {
  color: #909399;
  margin: 0;
}
.stats {
  margin: 16px 0;
}
.stat-card {
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
}
.stat-card:hover {
  transform: translateY(-3px);
}
.stat {
  text-align: center;
  padding: 8px 0;
}
.num {
  font-size: 28px;
  font-weight: 700;
  color: #409eff;
}
.label {
  color: #909399;
  font-size: 13px;
  margin-top: 4px;
}
</style>

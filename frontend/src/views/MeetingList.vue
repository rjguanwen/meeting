<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>会议管理</span>
        <el-button v-if="auth.isAdmin" type="primary" :icon="Plus" @click="openCreate">创建会议</el-button>
      </div>
    </template>

    <el-alert
      v-if="auth.isLeader"
      title="请选择筹备中的会议，进入「汇报事项录入」提交本组织需要汇报的内容。会议开始后不可再录入。"
      type="info"
      :closable="false"
      show-icon
      class="tip"
    />

    <!-- 查询区 -->
    <el-form :inline="true" class="search-form" @submit.prevent="doSearch">
      <el-form-item label="会议名称">
        <el-input
          v-model="query.keyword"
          placeholder="输入会议名称关键词"
          clearable
          style="width: 220px"
          @keyup.enter="doSearch"
          @clear="doSearch"
        />
      </el-form-item>
      <el-form-item label="会议时间">
        <el-date-picker
          v-model="query.range"
          type="daterange"
          range-separator="至"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          value-format="YYYY-MM-DD"
          style="width: 260px"
        />
      </el-form-item>
      <el-form-item label="会议状态">
        <el-select v-model="query.status" placeholder="全部状态" clearable style="width: 130px" @change="doSearch">
          <el-option label="筹备中" value="draft" />
          <el-option label="进行中" value="ongoing" />
          <el-option label="已结束" value="finished" />
          <el-option label="已归档" value="archived" />
        </el-select>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" :icon="Search" @click="doSearch">查询</el-button>
        <el-button :icon="RefreshLeft" @click="resetSearch">重置</el-button>
      </el-form-item>
    </el-form>

    <el-table :data="meetings" border v-loading="loading">
      <el-table-column label="会议名称" min-width="200">
        <template #default="{ row }">
          <el-link type="primary" :underline="false" @click="goDetail(row)">{{ row.title }}</el-link>
        </template>
      </el-table-column>
      <el-table-column label="会议时间" width="160">
        <template #default="{ row }">{{ fmtTime(row.meeting_time) }}</template>
      </el-table-column>
      <el-table-column label="会议地点" min-width="140">
        <template #default="{ row }">{{ row.location || '—' }}</template>
      </el-table-column>
      <el-table-column label="保密" width="80">
        <template #default="{ row }">
          <el-tag v-if="row.is_confidential" type="danger" size="small">保密</el-tag>
          <span v-else class="plain">公开</span>
        </template>
      </el-table-column>
      <el-table-column label="参会组织" min-width="200">
        <template #default="{ row }">
          {{ (row.orgs || []).map((o) => o.org?.name).filter(Boolean).join('、') || '—' }}
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)">{{ statusText(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="220" fixed="right">
        <template #default="{ row }">
          <el-button size="small" type="primary" @click="goDetail(row)">详情</el-button>
          <el-button
            v-if="row.status === 'draft' && (auth.isAdmin || auth.isLeader)"
            size="small"
            type="success"
            @click="goEntry(row)"
          >
            录入汇报
          </el-button>
          <el-button v-if="auth.isAdmin && row.status !== 'archived'" size="small" type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-if="!loading && !meetings.length" description="暂无符合条件的会议" />

    <!-- 分页 -->
    <div v-if="total > 0" class="pagination">
      <el-pagination
        background
        layout="total, sizes, prev, pager, next, jumper"
        :total="total"
        :current-page="page"
        :page-size="pageSize"
        :page-sizes="[10, 20, 50]"
        @current-change="onPageChange"
        @size-change="onSizeChange"
      />
    </div>

    <!-- 创建会议弹框 -->
    <MeetingFormDialog
      v-model="createVisible"
      title="创建会议"
      :initial="createInitial"
      :loading="saving"
      @submit="createMeeting"
    />
  </el-card>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, Search, RefreshLeft } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { meetingApi } from '../api'
import { useAuthStore } from '../stores/auth'
import MeetingFormDialog from '../components/MeetingFormDialog.vue'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const meetings = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const loading = ref(false)
const query = reactive({ keyword: '', range: null, status: '' })

async function load() {
  loading.value = true
  try {
    const params = {
      page: page.value,
      page_size: pageSize.value,
    }
    if (query.keyword) params.keyword = query.keyword
    if (query.range && query.range.length === 2) {
      params.start = query.range[0]
      params.end = query.range[1]
    }
    if (query.status) params.status = query.status
    const data = await meetingApi.list(params)
    meetings.value = data.items
    total.value = data.total
  } finally {
    loading.value = false
  }
}

function doSearch() {
  page.value = 1
  load()
}

function resetSearch() {
  query.keyword = ''
  query.range = null
  query.status = ''
  page.value = 1
  load()
}

function onPageChange(p) {
  page.value = p
  load()
}

function onSizeChange(size) {
  pageSize.value = size
  page.value = 1
  load()
}

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
function goEntry(row) {
  router.push(`/entry/${row.id}`)
}
const createVisible = ref(false)
const saving = ref(false)
// 创建时默认会议时间为明天 09:00
const createInitial = computed(() => ({
  title: '',
  location: '',
  meeting_time: defaultTime(),
  is_confidential: false,
}))

function defaultTime() {
  const d = new Date()
  d.setDate(d.getDate() + 1)
  d.setHours(9, 0, 0, 0)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

function openCreate() {
  createVisible.value = true
}

async function createMeeting(payload) {
  saving.value = true
  try {
    const m = await meetingApi.create(payload)
    ElMessage.success('会议已创建')
    createVisible.value = false
    router.push(`/meetings/${m.id}`)
  } finally {
    saving.value = false
  }
}

function remove(row) {
  ElMessageBox.confirm(`确定删除会议「${row.title}」吗？该操作不可恢复。`, '提示', { type: 'warning' })
    .then(async () => {
      await meetingApi.remove(row.id)
      ElMessage.success('已删除')
      await load()
    })
    .catch(() => {})
}

// 支持从首页统计卡片带状态进入：如 /meetings?status=ongoing
onMounted(() => {
  const s = route.query.status
  if (typeof s === 'string' && ['draft', 'ongoing', 'finished', 'archived'].includes(s)) {
    query.status = s
  }
  load()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.tip {
  margin-bottom: 14px;
}
.search-form {
  margin-bottom: 4px;
}
.pagination {
  margin-top: 14px;
  display: flex;
  justify-content: flex-end;
}
.plain {
  color: #909399;
}
</style>

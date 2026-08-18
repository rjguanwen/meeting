<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>操作日志</span>
      </div>
    </template>

    <el-alert
      title="系统自动记录登录日志与关键操作日志（会议、汇报事项、讨论结论、附件、组织、账号等），按时间倒序展示。"
      type="info"
      :closable="false"
      show-icon
      class="tip"
    />

    <!-- 查询区 -->
    <el-form :inline="true" class="search-form" @submit.prevent="doSearch">
      <el-form-item label="关键词">
        <el-input
          v-model="query.keyword"
          placeholder="用户名 / 操作详情"
          clearable
          style="width: 200px"
          @keyup.enter="doSearch"
          @clear="doSearch"
        />
      </el-form-item>
      <el-form-item label="操作类型">
        <el-select v-model="query.action" placeholder="全部" clearable filterable style="width: 180px" @change="doSearch">
          <el-option v-for="(label, val) in actionTextMap" :key="val" :label="label" :value="val" />
        </el-select>
      </el-form-item>
      <el-form-item label="目标类型">
        <el-select v-model="query.target_type" placeholder="全部" clearable style="width: 130px" @change="doSearch">
          <el-option label="会议" value="meeting" />
          <el-option label="汇报事项" value="item" />
          <el-option label="讨论结论" value="conclusion" />
          <el-option label="附件" value="attachment" />
          <el-option label="组织" value="org" />
          <el-option label="账号" value="user" />
        </el-select>
      </el-form-item>
      <el-form-item label="时间范围">
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
      <el-form-item>
        <el-button type="primary" :icon="Search" @click="doSearch">查询</el-button>
        <el-button :icon="RefreshLeft" @click="resetSearch">重置</el-button>
      </el-form-item>
    </el-form>

    <el-table :data="logs" border v-loading="loading">
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="username" label="用户名" width="120" />
      <el-table-column label="操作类型" width="130">
        <template #default="{ row }">
          <el-tag size="small" :type="tagType(row.action)">{{ actionText(row.action) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="目标" width="90">
        <template #default="{ row }">{{ targetText(row.target_type) }}</template>
      </el-table-column>
      <el-table-column prop="detail" label="操作详情" min-width="260" show-overflow-tooltip />
      <el-table-column prop="ip" label="IP" width="130" />
      <el-table-column label="时间" width="165">
        <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
      </el-table-column>
    </el-table>

    <el-empty v-if="!loading && !logs.length" description="暂无日志记录" />

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
  </el-card>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { Search, RefreshLeft } from '@element-plus/icons-vue'
import { logApi } from '../api'

const logs = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const query = reactive({ keyword: '', action: '', target_type: '', range: null })

const actionTextMap = {
  login: '登录',
  login_fail: '登录失败',
  'org.create': '创建组织',
  'org.update': '修改组织',
  'org.delete': '删除组织',
  'user.create': '创建账号',
  'user.update': '修改账号',
  'meeting.create': '创建会议',
  'meeting.update': '修改会议',
  'meeting.delete': '删除会议',
  'meeting.set_orgs': '设置参会组织',
  'meeting.start': '开始会议',
  'meeting.finish': '结束会议',
  'meeting.archive': '归档会议',
  'meeting.material': '生成材料',
  'meeting.minutes': '生成纪要',
  'item.create': '录入汇报事项',
  'item.update': '修改汇报事项',
  'item.delete': '删除汇报事项',
  'conclusion.create': '添加结论/任务',
  'conclusion.update': '修改结论/任务',
  'conclusion.delete': '删除结论/任务',
  'attachment.upload': '上传附件',
  'attachment.delete': '删除附件',
}

function actionText(a) {
  return actionTextMap[a] || a
}

function tagType(a) {
  if (a === 'login') return 'success'
  if (a === 'login_fail') return 'danger'
  if (a.startsWith('meeting') || a === 'meeting.material' || a === 'meeting.minutes') return 'primary'
  if (a.startsWith('attachment') || a.startsWith('conclusion')) return 'warning'
  return 'info'
}

function targetText(t) {
  return { meeting: '会议', item: '汇报事项', conclusion: '结论', attachment: '附件', org: '组织', user: '账号' }[t] || t || '—'
}

function fmtTime(t) {
  if (!t) return '—'
  return t.slice(0, 16).replace('T', ' ')
}

async function load() {
  loading.value = true
  try {
    const params = { page: page.value, page_size: pageSize.value }
    if (query.keyword) params.keyword = query.keyword
    if (query.action) params.action = query.action
    if (query.target_type) params.target_type = query.target_type
    if (query.range && query.range.length === 2) {
      params.start = query.range[0]
      params.end = query.range[1]
    }
    const data = await logApi.list(params)
    logs.value = data.items
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
  query.action = ''
  query.target_type = ''
  query.range = null
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

onMounted(load)
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
</style>

<template>
  <div v-if="meeting" class="entry">
    <el-card shadow="never">
      <template #header>
        <div class="header">
          <div>
            <h2 class="title">汇报事项录入 — {{ meeting.title }}</h2>
            <div class="meta">
              <el-tag :type="statusType" size="small">{{ statusText }}</el-tag>
              <span>会议时间：{{ fmtTime(meeting.meeting_time) }}</span>
              <span v-if="auth.isLeader && auth.user?.org">我的组织：{{ auth.user.org.name }}</span>
            </div>
          </div>
          <div class="actions">
            <el-button @click="$router.back()">返回</el-button>
            <el-button type="success" @click="$router.push(`/meetings/${meeting.id}`)">查看会议</el-button>
          </div>
        </div>
      </template>

      <el-alert
        v-if="!canEdit"
        :title="meeting.status === 'archived' ? '会议已归档，汇报事项只读。' : '会议已开始，汇报事项不可再修改。'"
        type="warning"
        :closable="false"
        show-icon
        class="tip"
      />
      <el-alert
        v-else
        :title="isCreator ? '选择参会组织后，为该组织录入需要向会议汇报的事项。' : '录入本组织需要向会议汇报的事项，点击「添加汇报事项」即可新增。'"
        type="info"
        :closable="false"
        show-icon
        class="tip"
      />
    </el-card>

    <el-card shadow="never">
      <template #header>
        <div class="block-header">
          <span>我的汇报事项</span>
          <el-button v-if="canEdit" type="primary" :icon="Plus" @click="openCreate">
            添加汇报事项
          </el-button>
        </div>
      </template>

      <el-table :data="myItems" border>
        <el-table-column type="index" label="#" width="50" />
        <el-table-column prop="title" label="事项标题" min-width="200" />
        <el-table-column prop="content" label="汇报内容" min-width="260" show-overflow-tooltip />
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 'presented' ? 'success' : 'info'">
              {{ row.status === 'presented' ? '已展示' : '待展示' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="附件" width="90">
          <template #default="{ row }">
            <el-button size="small" :icon="Paperclip" @click="openAttachments(row)">附件</el-button>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button size="small" :disabled="!canEdit" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" :disabled="!canEdit" @click="remove(row)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!myItems.length" description="尚未录入汇报事项" />
    </el-card>

    <!-- 附件管理 -->
    <el-dialog v-model="attDialog" :title="`附件管理 — ${currentItem?.title || ''}`" width="560px">
      <AttachmentManager v-if="currentItem" ref="attManagerRef" :item-id="currentItem.id" :disabled="!canEdit" />
    </el-dialog>

    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑汇报事项' : '添加汇报事项'" width="600px">
      <el-form :model="form" label-width="80px">
        <el-form-item label="事项标题" required>
          <el-input v-model="form.title" placeholder="请输入事项标题" maxlength="100" show-word-limit />
        </el-form-item>
        <el-form-item label="汇报内容">
          <el-input
            v-model="form.content"
            type="textarea"
            :rows="6"
            placeholder="请输入本次会议的汇报内容、进展、风险或需协调事项等"
          />
        </el-form-item>
        <el-form-item v-if="isCreator" label="录入组织" required>
          <el-select v-model="editOrgId" placeholder="选择要挂载的参会组织" style="width: 100%">
            <el-option v-for="o in meeting?.orgs || []" :key="o.org_id" :label="o.org?.name" :value="o.org_id" />
          </el-select>
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort_order" :min="0" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Plus, Paperclip } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { meetingApi, itemApi } from '../api'
import { useAuthStore } from '../stores/auth'
import AttachmentManager from '../components/AttachmentManager.vue'

const route = useRoute()
const auth = useAuthStore()
const meetingId = route.params.id

const meeting = ref(null)
const items = ref([])
const dialogVisible = ref(false)
const saving = ref(false)
const form = reactive({ id: null, title: '', content: '', sort_order: 0 })
// 会议创建者（含管理员）可为自己的会议录入汇报，需选择挂载的参会组织
const isCreator = computed(() => auth.isAdmin || meeting.value?.creator_id === auth.user?.id)
const editOrgId = ref(null)

// 附件管理
const attDialog = ref(false)
const currentItem = ref(null)
const attManagerRef = ref(null)

function openAttachments(row) {
  currentItem.value = row
  attDialog.value = true
  setTimeout(() => attManagerRef.value?.load(), 50)
}

const statusText = computed(
  () => ({ draft: '筹备中', ongoing: '进行中', finished: '已结束', archived: '已归档' })[meeting.value?.status] || '',
)
const statusType = computed(
  () => ({ draft: 'info', ongoing: 'warning', finished: 'success', archived: 'danger' })[meeting.value?.status] || 'info',
)
// 仅筹备中的会议允许录入/修改汇报事项
const canEdit = computed(() => meeting.value?.status === 'draft')

// 管理员看到全部，负责人只看到本组织
const myItems = computed(() => {
  if (auth.isAdmin) return items.value
  const orgId = auth.user?.org_id
  return items.value.filter((it) => it.org_id === orgId)
})

function fmtTime(t) {
  if (!t) return '—'
  return t.slice(0, 16).replace('T', ' ')
}

async function load() {
  meeting.value = await meetingApi.get(meetingId)
  items.value = await itemApi.list(meetingId)
  if (!editOrgId.value && meeting.value?.orgs?.length) {
    editOrgId.value = meeting.value.orgs[0].org_id
  }
}

function openCreate() {
  Object.assign(form, { id: null, title: '', content: '', sort_order: 0 })
  dialogVisible.value = true
}

function openEdit(row) {
  Object.assign(form, { id: row.id, title: row.title, content: row.content, sort_order: row.sort_order })
  dialogVisible.value = true
}

async function save() {
  if (!form.title.trim()) {
    ElMessage.warning('请输入事项标题')
    return
  }
  const orgId = isCreator.value ? editOrgId.value : auth.user?.org_id
  if (!orgId) {
    ElMessage.warning('请先选择录入组织')
    return
  }
  saving.value = true
  try {
    const payload = { org_id: orgId, title: form.title, content: form.content, sort_order: form.sort_order }
    if (form.id) {
      await itemApi.update(form.id, payload)
    } else {
      await itemApi.create(meetingId, payload)
    }
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

function remove(row) {
  ElMessageBox.confirm(`确定删除「${row.title}」吗？`, '提示', { type: 'warning' })
    .then(async () => {
      await itemApi.remove(row.id)
      ElMessage.success('已删除')
      await load()
    })
    .catch(() => {})
}

onMounted(load)
</script>

<style scoped>
.entry {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}
.title {
  margin: 0 0 8px;
}
.meta {
  display: flex;
  gap: 12px;
  color: #909399;
  font-size: 13px;
  align-items: center;
}
.actions {
  display: flex;
  gap: 8px;
}
.block-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.tip {
  margin-bottom: 8px;
}
</style>

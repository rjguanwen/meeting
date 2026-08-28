<template>
  <div v-if="meeting" class="detail">
    <el-card shadow="never">
      <template #header>
        <div class="header">
          <div>
            <h2 class="title">{{ meeting.title }}</h2>
            <div class="meta">
              <el-tag :type="statusType" size="small">{{ statusText }}</el-tag>
              <el-tag v-if="meeting.is_confidential" type="danger" size="small">保密会议</el-tag>
              <span>会议时间：{{ fmtTime(meeting.meeting_time) }}</span>
              <span v-if="meeting.location">会议地点：{{ meeting.location }}</span>
              <span>{{ meeting.description }}</span>
            </div>
          </div>
          <div class="actions">
            <el-button @click="router.push('/meetings')">返回</el-button>
            <el-button v-if="auth.isAdmin && meeting.status !== 'archived'" :icon="Edit" @click="openEdit">编辑</el-button>
            <el-button
              v-if="meeting.status === 'draft' && (auth.isAdmin || meeting.creator_id === auth.user?.id)"
              type="success"
              @click="$router.push(`/entry/${meeting.id}`)"
            >
              录入汇报
            </el-button>
            <el-button type="warning" @click="enterShow">
              会议展示
            </el-button>
          </div>
        </div>
      </template>

      <el-descriptions :column="2" border>
        <el-descriptions-item label="参会组织">
          {{ orgNames.join('、') || '未设置' }}
        </el-descriptions-item>
        <el-descriptions-item label="汇报事项数">{{ items.length }} 条</el-descriptions-item>
      </el-descriptions>

      <!-- 状态流转（管理员） -->
      <div v-if="auth.isAdmin" class="status-bar">
        <span class="status-label">会议状态操作：</span>
        <el-button v-if="meeting.status === 'draft'" type="primary" :icon="VideoPlay" @click="startMeeting">
          开始会议
        </el-button>
        <el-button v-if="meeting.status === 'ongoing'" type="warning" :icon="VideoPause" @click="finishMeeting">
          结束会议
        </el-button>
        <el-button
          v-if="meeting.status !== 'archived'"
          type="danger"
          :icon="FolderChecked"
          @click="archiveMeeting"
        >
          会议归档
        </el-button>
        <el-tag v-if="meeting.status === 'archived'" type="danger">已归档（材料与纪要只读）</el-tag>
      </div>
    </el-card>

    <!-- 管理员：参会组织设置 -->
    <el-card v-if="auth.isAdmin" shadow="never">
      <template #header>
        <div class="block-header">
          <span>参会组织</span>
          <el-tooltip :disabled="meeting.status === 'draft'" :content="orgSettingTip" placement="top">
            <span>
              <el-button size="small" :icon="Setting" :disabled="meeting.status !== 'draft'" @click="openOrgDialog">
                设置参会组织
              </el-button>
            </span>
          </el-tooltip>
        </div>
      </template>
      <div v-if="orgList.length" class="org-tags">
        <el-tag v-for="o in orgList" :key="o.id" :type="o.org?.type === 'team' ? 'warning' : 'primary'" size="large">
          {{ o.org?.name }}
        </el-tag>
      </div>
      <el-empty v-else description="尚未设置参会组织" :image-size="60" />
    </el-card>

    <!-- 汇报事项 -->
    <el-card shadow="never">
      <template #header>
        <div class="block-header">
          <span>汇报事项</span>
          <el-tooltip :disabled="!materialDisabledTip" :content="materialDisabledTip" placement="top">
            <span>
              <el-button
                v-if="auth.isAdmin"
                size="small"
                type="primary"
                :icon="Document"
                :disabled="!canGenerateMaterial"
                @click="generateMaterial"
              >
                {{ materialBtnText }}
              </el-button>
            </span>
          </el-tooltip>
        </div>
      </template>

      <div v-for="grp in groupedItems" :key="grp.org.id" class="org-block">
        <div class="org-title">
          <el-icon><OfficeBuilding /></el-icon>
          {{ grp.org.name }}
          <span class="count">（{{ grp.items.length }} 项）</span>
        </div>
        <el-collapse>
          <el-collapse-item v-for="item in grp.items" :key="item.id">
            <template #title>
              <div class="item-title">
                <span>{{ item.title }}</span>
                <el-tag size="small" :type="item.status === 'presented' ? 'success' : 'info'">
                  {{ item.status === 'presented' ? '已展示' : '待展示' }}
                </el-tag>
              </div>
            </template>
            <div class="item-content">
              <p class="content-text">{{ item.content || '（无详细内容）' }}</p>
              <div class="item-meta">录入人：{{ item.creator?.name || '—' }}</div>

              <template v-if="concByItem[item.id]?.length">
                <el-divider content-position="left">讨论结论 / 任务</el-divider>
                <div v-for="cc in concByItem[item.id]" :key="cc.id" class="conclusion">
                  <el-tag size="small" :type="cc.kind === 'action' ? 'danger' : 'success'">
                    {{ cc.kind === 'action' ? '新任务' : '结论' }}
                  </el-tag>
                  <span class="cc-text">{{ cc.content }}</span>
                  <span v-if="cc.owner" class="cc-meta">负责人：{{ cc.owner }}</span>
                  <span v-if="cc.due_date" class="cc-meta">截止：{{ cc.due_date }}</span>
                </div>
              </template>
              <div v-else class="no-conc">暂无讨论结论</div>

              <el-divider content-position="left">附件</el-divider>
              <AttachmentManager :item-id="item.id" disabled />
            </div>
          </el-collapse-item>
        </el-collapse>
      </div>
      <el-empty v-if="!items.length" description="暂无汇报事项" />
    </el-card>

    <!-- 会议纪要 -->
    <el-card shadow="never">
      <template #header>
        <div class="block-header">
          <span>会议纪要</span>
          <div>
            <el-tooltip :disabled="!minutesDisabledTip" :content="minutesDisabledTip" placement="top">
              <span>
                <el-button
                  v-if="auth.isAdmin"
                  size="small"
                  type="primary"
                  :icon="DocumentChecked"
                  :disabled="!canGenerateMinutes || editingMinutes"
                  @click="generateMinutes"
                >
                  {{ minutesBtnText }}
                </el-button>
              </span>
            </el-tooltip>
            <el-button v-if="canEditMinutes" size="small" type="warning" :icon="Edit" @click="startEditMinutes">
              编辑纪要
            </el-button>
            <template v-if="editingMinutes">
              <el-button size="small" type="primary" :icon="Check" :loading="savingMinutes" @click="saveMinutes">
                保存
              </el-button>
              <el-button size="small" @click="cancelEditMinutes">取消</el-button>
            </template>
            <el-button v-if="minutes.content && !editingMinutes" size="small" type="success" @click="downloadMinutes">
              下载 .md
            </el-button>
          </div>
        </div>
      </template>
      <div v-if="minutes.content && !editingMinutes" class="minutes" v-html="renderedMinutes"></div>
      <el-input
        v-else-if="editingMinutes"
        v-model="editContent"
        type="textarea"
        :rows="20"
        class="minutes-editor"
        placeholder="会议纪要（Markdown 格式）"
      />
      <el-empty v-else description="会议纪要尚未生成" :image-size="60" />
    </el-card>

    <!-- 编辑会议弹框 -->
    <MeetingFormDialog
      v-model="editVisible"
      title="编辑会议"
      :initial="editInitial"
      :allow-orgs="meeting.status === 'draft'"
      :loading="savingMeeting"
      @submit="saveMeeting"
    />

    <!-- 参会组织设置对话框 -->
    <el-dialog v-model="orgDialog" title="设置参会组织" width="480px">
      <el-alert title="仅部门可直接选择；选择小组时需先选所属部门。" type="info" :closable="false" class="tip" />
      <el-select v-model="selectedOrgs" multiple filterable placeholder="选择参会组织" style="width: 100%" class="mt">
        <el-option-group v-for="d in orgTree" :key="d.id" :label="d.name">
          <el-option :label="d.name" :value="d.id" />
          <el-option v-for="t in d.children || []" :key="t.id" :label="`↳ ${t.name}`" :value="t.id" />
        </el-option-group>
      </el-select>
      <template #footer>
        <el-button @click="orgDialog = false">取消</el-button>
        <el-button type="primary" :loading="savingOrgs" @click="saveOrgs">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Setting,
  Document,
  DocumentChecked,
  Edit,
  Check,
  VideoPlay,
  VideoPause,
  FolderChecked,
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { marked } from 'marked'
import { meetingApi, itemApi, conclusionApi, minutesApi, orgApi } from '../api'
import { useAuthStore } from '../stores/auth'
import AttachmentManager from '../components/AttachmentManager.vue'
import MeetingFormDialog from '../components/MeetingFormDialog.vue'

marked.setOptions({ gfm: true, breaks: true })

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const meetingId = route.params.id

const meeting = ref(null)
const items = ref([])
const conclusions = ref([])
const minutes = ref({})
const orgTree = ref([])
const orgDialog = ref(false)
const selectedOrgs = ref([])
const savingOrgs = ref(false)
const editingMinutes = ref(false)
const editContent = ref('')
const savingMinutes = ref(false)

const statusText = computed(
  () => ({ draft: '筹备中', ongoing: '进行中', finished: '已结束', archived: '已归档' })[meeting.value?.status] || '',
)
const statusType = computed(
  () => ({ draft: 'info', ongoing: 'warning', finished: 'success', archived: 'danger' })[meeting.value?.status] || 'info',
)
const orgList = computed(() => meeting.value?.orgs || [])
const orgNames = computed(() => orgList.value.map((o) => o.org?.name).filter(Boolean))
const groupedItems = computed(() => {
  const map = new Map()
  for (const it of items.value) {
    const org = it.org
    if (!org) continue
    if (!map.has(org.id)) map.set(org.id, { org, items: [] })
    map.get(org.id).items.push(it)
  }
  return [...map.values()]
})
const concByItem = computed(() => {
  const map = {}
  for (const cc of conclusions.value) {
    if (!map[cc.report_item_id]) map[cc.report_item_id] = []
    map[cc.report_item_id].push(cc)
  }
  return map
})

// 参会组织设置：仅筹备中允许
const orgSettingTip = computed(() => {
  const st = meeting.value?.status
  if (st === 'draft') return ''
  if (st === 'archived') return '会议已归档，不能修改参会组织'
  return '会议已开始，不能再设置参会组织'
})

// 材料生成权限：会前(draft)可多次；开始后仅一次（material_generated）；归档后禁止
const canGenerateMaterial = computed(() => {
  const st = meeting.value?.status
  if (st === 'archived') return false
  if (st === 'draft') return true
  return !meeting.value?.material_generated
})
const materialBtnText = computed(() => {
  const st = meeting.value?.status
  if (st === 'archived') return '材料已归档'
  if (st === 'draft') return '生成会议材料（会前可多次）'
  if (meeting.value?.material_generated) return '正式材料已生成'
  return '生成正式会议材料'
})
const materialDisabledTip = computed(() => {
  const st = meeting.value?.status
  if (st === 'archived') return '会议已归档，材料只读'
  if (st !== 'draft' && meeting.value?.material_generated) return '会议开始后仅可生成一次正式材料'
  return ''
})

// 纪要渲染为 HTML（Markdown）
const renderedMinutes = computed(() =>
  minutes.value.content ? marked.parse(minutes.value.content) : '',
)

// 纪要生成权限：仅会议开始后（进行中/已结束）且归档前可用
const canGenerateMinutes = computed(() => {
  const st = meeting.value?.status
  return st === 'ongoing' || st === 'finished'
})
const minutesBtnText = computed(() => {
  const st = meeting.value?.status
  if (st === 'archived') return '纪要已归档'
  if (st === 'draft') return '会议开始后可生成纪要'
  return '生成会议纪要'
})
const minutesDisabledTip = computed(() => {
  const st = meeting.value?.status
  if (st === 'archived') return '会议已归档，纪要只读'
  if (st === 'draft') return '会议开始后才可以生成会议纪要'
  return ''
})

// 纪要编辑权限：会议结束后（进行中/已结束）可编辑，归档后只读；仅管理员
const canEditMinutes = computed(() => {
  const st = meeting.value?.status
  return auth.isAdmin && (st === 'ongoing' || st === 'finished') && !!minutes.value.content
})

function fmtTime(t) {
  if (!t) return '—'
  return t.slice(0, 16).replace('T', ' ')
}

async function load() {
  meeting.value = await meetingApi.get(meetingId)
  items.value = await itemApi.list(meetingId)
  conclusions.value = await conclusionApi.list(meetingId)
  const m = await minutesApi.get(meetingId)
  if (m && m.content) minutes.value = m
}

function enterShow() {
  router.push(`/show/${meetingId}`)
}

async function generateMaterial() {
  await meetingApi.material(meetingId)
  ElMessage.success('会议材料已生成')
  await load()
}

async function generateMinutes() {
  await minutesApi.generate(meetingId)
  ElMessage.success('会议纪要已生成')
  await load()
}

function startEditMinutes() {
  editContent.value = minutes.value.content
  editingMinutes.value = true
}

function cancelEditMinutes() {
  editingMinutes.value = false
  editContent.value = ''
}

async function saveMinutes() {
  savingMinutes.value = true
  try {
    await minutesApi.update(meetingId, editContent.value)
    ElMessage.success('会议纪要已保存')
    await load()
  } finally {
    editingMinutes.value = false
    savingMinutes.value = false
  }
}

async function startMeeting() {
  if (!items.value.length) {
    ElMessage.warning('该会议尚未录入任何汇报内容，无法开始会议')
    return
  }
  try {
    await ElMessageBox.confirm('请确认所有参会组织的汇报内容已全部录入完成，是否开始会议？', '开始会议', {
      confirmButtonText: '确认开始',
      cancelButtonText: '再检查一下',
      type: 'warning',
    })
  } catch {
    return
  }
  await meetingApi.start(meetingId)
  ElMessage.success('会议已开始，进入会议展示')
  router.push(`/show/${meetingId}`)
}

async function finishMeeting() {
  await meetingApi.finish(meetingId)
  ElMessage.success('会议已结束')
  await load()
}

// 归档：若尚未生成纪要且当前可生成（进行中/已结束），询问是否先生成
async function archiveMeeting() {
  if (!minutes.value.content && canGenerateMinutes.value) {
    try {
      const action = await ElMessageBox.confirm(
        '会议尚未生成纪要，是否先生成纪要后再归档？',
        '归档确认',
        {
          confirmButtonText: '先生成纪要',
          cancelButtonText: '直接归档',
          distinguishCancelAndClose: true,
          type: 'warning',
        },
      )
      await minutesApi.generate(meetingId)
      ElMessage.success('纪要已生成')
    } catch (e) {
      if (e !== 'cancel') {
        return // 关闭对话框，取消归档
      }
    }
  }
  await meetingApi.archive(meetingId)
  ElMessage.success('会议已归档')
  await load()
}

function downloadMinutes() {
  const blob = new Blob([minutes.value.content], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${meeting.value?.title || '会议纪要'}.md`
  a.click()
  URL.revokeObjectURL(url)
}

// 编辑会议：名称/地点/时间随时可改（非归档），参会组织仅筹备中可改
const editVisible = ref(false)
const savingMeeting = ref(false)
const editInitial = computed(() => ({
  title: meeting.value?.title || '',
  location: meeting.value?.location || '',
  meeting_time: toFullTime(meeting.value?.meeting_time),
  is_confidential: meeting.value?.is_confidential || false,
  org_ids: orgList.value.map((o) => o.org_id),
}))

function toFullTime(t) {
  if (!t) return ''
  // 后端输出为 YYYY-MM-DD HH:mm，补齐秒以匹配 date-picker 格式
  return t.length === 16 ? `${t}:00` : t
}

function openEdit() {
  editVisible.value = true
}

async function saveMeeting(payload) {
  savingMeeting.value = true
  try {
    await meetingApi.update(meetingId, payload)
    ElMessage.success('会议已更新')
    editVisible.value = false
    await load()
  } finally {
    savingMeeting.value = false
  }
}

async function openOrgDialog() {
  orgTree.value = await orgApi.tree()
  selectedOrgs.value = orgList.value.map((o) => o.org_id)
  orgDialog.value = true
}

async function saveOrgs() {
  if (!selectedOrgs.value.length) {
    ElMessage.warning('请至少选择一个参会组织')
    return
  }
  savingOrgs.value = true
  try {
    await meetingApi.setOrgs(meetingId, selectedOrgs.value)
    ElMessage.success('参会组织已更新')
    orgDialog.value = false
    await load()
  } finally {
    savingOrgs.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.detail {
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
.status-bar {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px dashed #e4e7ed;
  display: flex;
  align-items: center;
  gap: 8px;
}
.status-label {
  color: #909399;
  font-size: 13px;
}
.block-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.org-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.org-block {
  margin-bottom: 16px;
}
.org-title {
  font-size: 15px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
}
.count {
  color: #909399;
  font-size: 12px;
  font-weight: normal;
}
.item-title {
  display: flex;
  gap: 8px;
  align-items: center;
}
.item-content {
  padding: 0 12px 8px;
}
.content-text {
  margin: 0 0 8px;
  white-space: pre-wrap;
}
.item-meta {
  color: #909399;
  font-size: 12px;
}
.conclusion {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 4px 0;
}
.cc-text {
  flex: 1;
}
.cc-meta {
  color: #909399;
  font-size: 12px;
}
.no-conc {
  color: #c0c4cc;
  font-size: 13px;
}
.minutes {
  background: #f8f9fa;
  border-radius: 6px;
  padding: 16px 20px;
  font-size: 14px;
  line-height: 1.8;
  max-height: 560px;
  overflow: auto;
  color: #303133;
}
.minutes-editor {
  font-family: 'JetBrains Mono', Consolas, Monaco, monospace;
  font-size: 13px;
  line-height: 1.7;
}
.minutes :deep(h1),
.minutes :deep(h2),
.minutes :deep(h3) {
  margin: 16px 0 10px;
  font-weight: 600;
  color: #1f2d3d;
}
.minutes :deep(h1:first-child),
.minutes :deep(h2:first-child),
.minutes :deep(h3:first-child) {
  margin-top: 0;
}
.minutes :deep(p) {
  margin: 8px 0;
}
.minutes :deep(ul),
.minutes :deep(ol) {
  margin: 8px 0;
  padding-left: 24px;
}
.minutes :deep(li) {
  margin: 4px 0;
}
.minutes :deep(strong) {
  font-weight: 600;
  color: #1f2d3d;
}
.minutes :deep(blockquote) {
  margin: 8px 0;
  padding: 6px 14px;
  border-left: 3px solid #409eff;
  background: #eef5fe;
  color: #606266;
  border-radius: 0 4px 4px 0;
}
.minutes :deep(code) {
  background: #eef1f4;
  border-radius: 3px;
  padding: 1px 5px;
  font-size: 13px;
}
.minutes :deep(table) {
  border-collapse: collapse;
  margin: 10px 0;
  width: 100%;
}
.minutes :deep(th),
.minutes :deep(td) {
  border: 1px solid #dcdfe6;
  padding: 6px 10px;
  text-align: left;
}
.minutes :deep(th) {
  background: #f0f2f5;
  font-weight: 600;
}
.mt {
  margin-top: 12px;
}
.tip {
  margin-bottom: 8px;
}
</style>

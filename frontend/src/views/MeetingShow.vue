<template>
  <div v-if="material" class="show-page">
    <!-- 顶栏 -->
    <div class="topbar">
      <div class="topbar-left">
        <el-icon :size="20" color="#409eff"><Monitor /></el-icon>
        <span class="meeting-title">{{ material.title }}</span>
        <span v-if="material.location" class="meeting-meta">地点：{{ material.location }}</span>
        <span v-if="material.meeting_time" class="meeting-meta">时间：{{ material.meeting_time }}</span>
      </div>
      <div class="topbar-right">
        <el-button size="small" @click="$router.push(`/meetings/${material.meeting_id}`)">退出展示</el-button>
      </div>
    </div>

    <!-- 幻灯片区 -->
    <div class="slide-stage">
      <div v-if="slide" class="slide">
        <!-- 页眉：组织信息 -->
        <div class="slide-header">
          <el-tag v-if="slide.org_type === 'team'" type="warning" size="small">小组</el-tag>
          <el-tag v-else type="primary" size="small">部门</el-tag>
          <span class="org-name">{{ slide.dept_name ? `${slide.dept_name} · ` : '' }}{{ slide.org_name }}</span>
          <span class="page-no">{{ slide.index }} / {{ material.total }}</span>
        </div>

        <!-- 内容区 -->
        <div class="slide-body">
          <h2 class="slide-title">{{ slide.title }}</h2>
          <div v-if="slide.content && slide.content.trim()" class="slide-content md-body" v-html="renderMarkdown(slide.content)"></div>
          <el-empty v-else description="（无详细汇报内容）" :image-size="60" />
          <div class="slide-reporter">汇报人：{{ slide.reporter || '—' }}</div>
        </div>

        <!-- 附件 -->
        <div v-if="slideAttachments.length" class="attachments">
          <div class="section-label">附件（{{ slideAttachments.length }}）</div>
          <div class="att-grid">
            <div v-for="att in slideAttachments" :key="att.id" class="att-cell">
              <img v-if="att.type === 'image' && att.url" :src="att.url" class="att-img" @click="openPreview(att)" />
              <video v-else-if="att.type === 'video' && att.url" :src="att.url" controls class="att-video"></video>
              <audio v-else-if="att.type === 'audio' && att.url" :src="att.url" controls class="att-audio"></audio>
              <div v-else class="att-file" @click="downloadAttachment(att)">
                <el-icon :size="28" color="#409eff"><Document /></el-icon>
                <div class="att-fname">{{ att.file_name }}</div>
                <div class="att-fsize">{{ fmtSize(att.file_size) }}</div>
                <el-button link type="primary" size="small">
                  {{ att.type === 'pdf' ? '预览' : '下载' }}
                </el-button>
              </div>
            </div>
          </div>
        </div>

        <!-- 已有结论 -->
        <div v-if="conclusions.length" class="existing-conclusions">
          <div class="section-label">已有讨论结论 / 任务</div>
          <div v-for="cc in conclusions" :key="cc.id" class="cc-row">
            <el-tag size="small" :type="cc.kind === 'action' ? 'danger' : 'success'">
              {{ cc.kind === 'action' ? '任务' : '结论' }}
            </el-tag>
            <span class="cc-text">{{ cc.content }}</span>
            <span v-if="cc.owner" class="cc-meta">负责人：{{ cc.owner }}</span>
            <span v-if="cc.due_date" class="cc-meta">截止：{{ cc.due_date }}</span>
            <el-button v-if="auth.isAdmin && canEditConclusion" link type="danger" size="small" @click="deleteConclusion(cc)">删除</el-button>
          </div>
        </div>

        <el-alert v-if="!canEditConclusion" :title="conclusionReadonlyTip" type="warning" :closable="false" show-icon class="archived-tip" />

        <!-- 结论录入 -->
        <div v-if="auth.isAdmin && canEditConclusion" class="conclusion-input">
          <div class="section-label">添加讨论结论 / 新任务</div>
          <div class="input-row">
            <el-radio-group v-model="newCC.kind">
              <el-radio-button class="radio-conclusion" value="conclusion">讨论结论</el-radio-button>
              <el-radio-button class="radio-action" value="action">新任务</el-radio-button>
            </el-radio-group>
            <el-input v-model="newCC.content" placeholder="输入讨论结论内容" style="flex: 1" maxlength="200" />
          </div>
          <div v-show="newCC.kind === 'action'" class="input-row action-row">
            <el-select
              v-model="newCC.owner"
              filterable
              teleported
              placeholder="选择负责人"
              style="width: 200px"
            >
              <el-option
                v-for="u in users"
                :key="u.id"
                :label="userLabel(u)"
                :value="u.name || u.username"
              />
            </el-select>
            <el-date-picker
              v-model="newCC.due_date"
              type="datetime"
              teleported
              placeholder="截止时间"
              value-format="YYYY-MM-DD HH:mm"
              @calendar-change="onCalendarChange"
              @change="onDueDateChange"
            />
          </div>
          <div class="input-row">
            <el-button type="success" :loading="addingCC" @click="addConclusion">保存结论</el-button>
          </div>
        </div>
      </div>

      <el-empty v-else description="该会议暂无汇报事项" />
    </div>

    <!-- 底部导航 -->
    <div class="bottombar">
      <el-button :icon="ArrowLeft" :disabled="current === 0" @click="prev">上一项</el-button>
      <div class="progress">{{ current + 1 }} / {{ material.total }}</div>
      <el-progress
        class="bar"
        :percentage="progressPct"
        :stroke-width="8"
        :show-text="false"
        color="#409eff"
      />
      <el-button :icon="ArrowRight" type="primary" :disabled="current === material.total - 1" @click="next">
        下一项
      </el-button>
    </div>

    <!-- 图片大图预览 -->
    <el-dialog v-model="imgPreviewVisible" width="auto" align-center :show-close="false" class="img-preview-dialog">
      <img v-if="imgPreviewUrl" :src="imgPreviewUrl" class="img-preview-full" />
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, ArrowRight, Document } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { meetingApi, conclusionApi, userApi } from '../api'
import { useAuthStore } from '../stores/auth'
import { isImage, isVideo, isAudio, isPdf, fmtSize, getFileUrl, downloadFile, openFileInNewTab } from '../utils/file'
import { renderMarkdown } from '../utils/md'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const meetingId = route.params.id

const material = ref(null)
const current = ref(0)
const allConclusions = ref([])
const addingCC = ref(false)
const users = ref([])
const newCC = reactive({ kind: 'conclusion', content: '', owner: '', due_date: null })

// 负责人下拉选项标签：姓名（组织）
function userLabel(u) {
  const org = u.org?.name
  const name = u.name || u.username
  return org ? `${name}（${org}）` : name
}

// 点击日期面板选中某日时，时间默认为 17:00
function onCalendarChange(dates) {
  if (!dates || dates.length === 0) return
  const d = dates[0]
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  newCC.due_date = `${y}-${m}-${day} 17:00`
}

// 兜底：若最终值为 00:00（未经过日历选择），补为 17:00
function onDueDateChange(val) {
  if (val && / 00:00$/.test(val)) {
    newCC.due_date = val.slice(0, 10) + ' 17:00'
  }
}

async function loadUsers() {
  if (auth.isAdmin) {
    try {
      users.value = await userApi.list()
    } catch {
      users.value = []
    }
  }
}

const isArchived = computed(() => material.value?.status === 'archived')
// 结论录入仅限进行中的会议
const canEditConclusion = computed(() => material.value?.status === 'ongoing')
const conclusionReadonlyTip = computed(() => {
  const st = material.value?.status
  if (st === 'archived') return '会议已归档，讨论结论只读，不能再录入。'
  if (st === 'finished') return '会议已结束，讨论结论只读，不能再录入。'
  return '会议尚未开始，讨论结论在会议进行中录入。'
})
const slide = computed(() => material.value?.slides?.[current.value] || null)
const progressPct = computed(() =>
  material.value?.total ? Math.round(((current.value + 1) / material.value.total) * 100) : 0,
)
const conclusions = computed(() =>
  slide.value ? allConclusions.value.filter((c) => c.report_item_id === slide.value.item_id) : [],
)

// 当前页附件：补充 type / url（异步加载）
const slideAttachments = ref([])

async function loadSlideAttachments() {
  const atts = slide.value?.attachments || []
  slideAttachments.value = atts.map((att) => {
    let type = 'file'
    if (isImage(att)) type = 'image'
    else if (isVideo(att)) type = 'video'
    else if (isAudio(att)) type = 'audio'
    else if (isPdf(att)) type = 'pdf'
    return { ...att, type, url: '' }
  })
  // 并行获取附件 URL（getFileUrl 内部有 urlCache，重复访问不重复下载）
  await Promise.all(
    slideAttachments.value.map(async (att) => {
      try {
        att.url = await getFileUrl(att.id)
      } catch {
        att.url = ''
      }
    }),
  )
}

async function load() {
  // 展示页只读获取材料（生成操作在详情页，遵守次数限制）；
  // 会议状态已随材料返回，无需再拉取整份会议详情。
  material.value = await meetingApi.materialView(meetingId)
  await Promise.all([loadConclusions(), loadSlideAttachments(), loadUsers()])
}

// 图片大图预览
const imgPreviewVisible = ref(false)
const imgPreviewUrl = ref('')
function openPreview(att) {
  imgPreviewUrl.value = att.url
  imgPreviewVisible.value = true
}

async function downloadAttachment(att) {
  if (att.type === 'pdf') {
    openFileInNewTab(att)
  } else {
    downloadFile(att)
  }
}

async function loadConclusions() {
  allConclusions.value = await conclusionApi.list(meetingId)
}

function next() {
  if (current.value < material.value.total - 1) {
    current.value++
    loadSlideAttachments()
  }
}
function prev() {
  if (current.value > 0) {
    current.value--
    loadSlideAttachments()
  }
}

async function addConclusion() {
  if (!newCC.content.trim()) {
    ElMessage.warning('请输入结论内容')
    return
  }
  if (newCC.kind === 'action' && !newCC.owner.trim()) {
    ElMessage.warning('新任务请填写负责人')
    return
  }
  addingCC.value = true
  try {
    await conclusionApi.create(meetingId, {
      report_item_id: slide.value.item_id,
      kind: newCC.kind,
      content: newCC.content,
      owner: newCC.owner,
      due_date: newCC.due_date,
    })
    ElMessage.success('已保存')
    Object.assign(newCC, { kind: 'conclusion', content: '', owner: '', due_date: null })
    await loadConclusions()
  } finally {
    addingCC.value = false
  }
}

function deleteConclusion(cc) {
  conclusionApi.remove(cc.id).then(() => {
    ElMessage.success('已删除')
    loadConclusions()
  })
}

// 是否在可输入控件内（输入框/文本域/下拉/可编辑元素），此时不响应翻页快捷键
function isTypingTarget(e) {
  const el = e.target
  if (!el || el === document.body) return false
  const tag = el.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  return el.isContentEditable === true
}

function onKeydown(e) {
  // 光标在输入框等控件中时，方向键/空格用于文本编辑，不触发翻页
  if (isTypingTarget(e) && e.key !== 'Escape') {
    return
  }
  if (e.key === 'Escape') {
    // ESC 退出展示，返回会议详情页
    exitShow()
    return
  }
  if (e.key === 'ArrowRight' || e.key === ' ') next()
  if (e.key === 'ArrowLeft') prev()
}

function exitShow() {
  router.push(`/meetings/${meetingId}`)
}

onMounted(() => {
  load()
  window.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>

<style scoped>
.show-page {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: #f0f2f5;
}
.topbar {
  height: 52px;
  background: #001529;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
}
.topbar-left {
  display: flex;
  align-items: center;
  gap: 10px;
}
.meeting-title {
  font-size: 16px;
  font-weight: 600;
}
.meeting-meta {
  font-size: 13px;
  color: #a0cfff;
}
.slide-stage {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  overflow: auto;
}
.slide {
  width: 100%;
  max-width: 1000px;
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.12);
  display: flex;
  flex-direction: column;
  max-height: 100%;
  overflow: auto;
  padding: 28px 36px;
  box-sizing: border-box;
}
.slide-header {
  display: flex;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid #f0f0f0;
  padding-bottom: 12px;
}
.org-name {
  font-size: 15px;
  font-weight: 600;
  flex: 1;
}
.page-no {
  color: #909399;
  font-size: 13px;
}
.slide-body {
  flex: 1;
  padding: 24px 0 12px;
}
.slide-title {
  font-size: 24px;
  color: #1f2d3d;
  margin: 0 0 16px;
}
.slide-content {
  font-size: 17px;
  line-height: 1.8;
  color: #303133;
  margin: 0;
}
.slide-reporter {
  margin-top: 16px;
  color: #909399;
  font-size: 13px;
}
.existing-conclusions {
  border-top: 1px dashed #e4e7ed;
  padding-top: 12px;
  margin-top: 12px;
}
.section-label {
  font-size: 13px;
  color: #606266;
  font-weight: 600;
  margin-bottom: 8px;
}
.cc-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  font-size: 14px;
}
.cc-text {
  flex: 1;
}
.cc-meta {
  color: #909399;
  font-size: 12px;
}
.conclusion-input {
  border-top: 1px dashed #e4e7ed;
  padding-top: 12px;
  margin-top: 12px;
}
.archived-tip {
  margin-top: 12px;
}
.attachments {
  border-top: 1px dashed #e4e7ed;
  padding-top: 12px;
  margin-top: 12px;
}
.att-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.att-cell {
  border: 1px solid #ebeef5;
  border-radius: 8px;
  padding: 8px;
  max-width: 300px;
}
.att-img {
  max-width: 280px;
  max-height: 180px;
  cursor: pointer;
  border-radius: 4px;
  display: block;
}
.att-video {
  max-width: 280px;
  max-height: 180px;
  display: block;
}
.att-audio {
  width: 280px;
  margin-top: 60px;
}
.att-file {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 8px;
  min-width: 140px;
  cursor: pointer;
}
.att-fname {
  font-size: 13px;
  color: #303133;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.att-fsize {
  font-size: 12px;
  color: #909399;
}
.img-preview-full {
  max-width: 80vw;
  max-height: 80vh;
}
.input-row {
  display: flex;
  gap: 10px;
  margin-bottom: 10px;
  align-items: center;
}
.action-row {
  margin-left: 4px;
}
/* 结论/新任务选项选中态均为绿色，与「保存结论」蓝色按钮区分 */
:deep(.radio-conclusion.is-active .el-radio-button__inner),
:deep(.radio-action.is-active .el-radio-button__inner) {
  color: #fff;
  background-color: #67c23a;
  border-color: #67c23a;
  box-shadow: -1px 0 0 0 #67c23a;
}
.bottombar {
  height: 64px;
  background: #fff;
  border-top: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 24px;
}
.progress {
  font-size: 13px;
  color: #909399;
  white-space: nowrap;
}
.bar {
  flex: 1;
}
</style>

<template>
  <div class="att-manager">
    <div v-if="list.length" class="att-list">
      <div v-for="att in list" :key="att.id" class="att-item">
        <el-icon class="att-icon" :color="iconColor(att)"><Document /></el-icon>
        <div class="att-info">
          <div class="att-name" :title="att.file_name">{{ att.file_name }}</div>
          <div class="att-size">{{ fmtSize(att.file_size) }}</div>
        </div>
        <el-button v-if="isPreviewable(att)" link type="primary" size="small" @click="preview(att)">预览</el-button>
        <el-button link size="small" @click="download(att)">下载</el-button>
        <el-button v-if="!disabled" link type="danger" size="small" @click="remove(att)">删除</el-button>
      </div>
    </div>
    <el-empty v-else description="暂无附件" :image-size="50" />

    <div v-if="!disabled" class="att-upload">
      <el-upload
        :show-file-list="false"
        :http-request="doUpload"
        accept=".jpg,.jpeg,.png,.gif,.webp,.bmp,.svg,.mp4,.mov,.avi,.mkv,.webm,.wmv,.flv,.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.md,.log,.csv,.rtf"
        :disabled="list.length >= 10 || uploading"
      >
        <el-button type="primary" plain :icon="UploadFilled" :loading="uploading" :disabled="list.length >= 10">
          {{ list.length >= 10 ? '已达 10 个上限' : '上传附件' }}
        </el-button>
      </el-upload>
      <span class="att-tip">最多 10 个附件，单个不超过 5MB，支持图片 / 视频 / PDF / Office / 文本</span>
      <el-progress
        v-if="uploading"
        :percentage="progress"
        :stroke-width="6"
        class="att-progress"
      />
    </div>

    <el-dialog v-model="previewVisible" :title="previewAtt?.file_name || '预览'" width="80%" top="5vh">
      <div class="preview-body">
        <img v-if="previewAtt && isImage(previewAtt) && previewUrl" :src="previewUrl" class="preview-img" />
        <video
          v-else-if="previewAtt && isVideo(previewAtt) && previewUrl"
          :src="previewUrl"
          controls
          autoplay
          class="preview-video"
        />
        <audio v-else-if="previewAtt && isAudio(previewAtt) && previewUrl" :src="previewUrl" controls class="preview-audio" />
        <iframe v-else-if="previewAtt && isPdf(previewAtt) && previewUrl" :src="previewUrl" class="preview-pdf"></iframe>
        <el-empty v-else description="该类型无法在线预览，请下载后查看" />
      </div>
      <template #footer>
        <el-button @click="previewVisible = false">关闭</el-button>
        <el-button v-if="previewAtt" type="primary" @click="download(previewAtt)">下载</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onBeforeUnmount, ref } from 'vue'
import { UploadFilled, Document } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { attachmentApi } from '../api'
import { fmtSize, isImage, isVideo, isPdf, isAudio, isPreviewable, getFileUrl, downloadFile } from '../utils/file'

const props = defineProps({
  itemId: { type: [Number, String], required: true },
  disabled: { type: Boolean, default: false },
})

const list = ref([])
const uploading = ref(false)
const progress = ref(0)
const previewVisible = ref(false)
const previewAtt = ref(null)
const previewUrl = ref('')

async function load() {
  list.value = await attachmentApi.list(props.itemId)
}

function iconColor(att) {
  if (isImage(att) || isVideo(att)) return '#409eff'
  if (isPdf(att)) return '#f56c6c'
  return '#909399'
}

async function doUpload({ file }) {
  if (file.size > 5 * 1024 * 1024) {
    ElMessage.error('单个附件不能超过 5MB')
    return
  }
  uploading.value = true
  progress.value = 0
  try {
    await attachmentApi.upload(props.itemId, file, (e) => {
      if (e.total) progress.value = Math.round((e.loaded / e.total) * 100)
    })
    ElMessage.success('上传成功')
    await load()
  } finally {
    uploading.value = false
  }
}

function remove(att) {
  ElMessageBox.confirm(`确定删除附件「${att.file_name}」吗？`, '提示', { type: 'warning' })
    .then(async () => {
      await attachmentApi.remove(att.id)
      ElMessage.success('已删除')
      await load()
    })
    .catch(() => {})
}

async function preview(att) {
  previewAtt.value = att
  previewVisible.value = true
  previewUrl.value = ''
  try {
    previewUrl.value = await getFileUrl(att.id)
  } catch {
    ElMessage.error('附件加载失败')
  }
}

function download(att) {
  downloadFile(att)
}

defineExpose({ load })

load()
</script>

<style scoped>
.att-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.att-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border: 1px solid #ebeef5;
  border-radius: 6px;
}
.att-icon {
  font-size: 22px;
}
.att-info {
  flex: 1;
  min-width: 0;
}
.att-name {
  font-size: 13px;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.att-size {
  font-size: 12px;
  color: #909399;
}
.att-upload {
  margin-top: 12px;
}
.att-tip {
  margin-left: 10px;
  color: #909399;
  font-size: 12px;
}
.att-progress {
  margin-top: 8px;
}
.preview-body {
  text-align: center;
  min-height: 200px;
}
.preview-img {
  max-width: 100%;
  max-height: 65vh;
}
.preview-video {
  max-width: 100%;
  max-height: 65vh;
}
.preview-audio {
  width: 80%;
  margin-top: 40px;
}
.preview-pdf {
  width: 100%;
  height: 65vh;
  border: none;
}
</style>

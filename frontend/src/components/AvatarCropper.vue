<template>
  <div class="cropper">
    <div class="crop-stage" ref="stageRef">
      <img
        v-if="imgSrc"
        ref="imgRef"
        :src="imgSrc"
        class="crop-img"
        :style="imgStyle"
        draggable="false"
        @load="onImgLoad"
      />
      <div v-if="imgSrc" class="crop-mask">
        <div class="crop-frame" />
      </div>
      <div v-if="!imgSrc" class="crop-empty">请选择图片</div>
    </div>

    <div class="crop-preview">
      <div class="preview-circle">
        <img
          v-if="imgSrc"
          :src="imgSrc"
          class="preview-img"
          :style="previewImgStyle"
          draggable="false"
        />
      </div>
      <span class="preview-tip">头像预览</span>
    </div>

    <div class="crop-controls">
      <el-slider v-model="scale" :min="1" :max="4" :step="0.01" :disabled="!imgSrc" />
      <div class="crop-actions">
        <el-button :disabled="!imgSrc" @click="pick">选择图片</el-button>
        <el-button type="primary" :disabled="!imgSrc" :loading="saving" @click="confirm">确定裁剪</el-button>
      </div>
      <input ref="fileRef" type="file" accept="image/*" class="hidden-input" @change="onFileChange" />
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'

const emit = defineEmits(['confirm'])

const stageRef = ref(null)
const imgRef = ref(null)
const fileRef = ref(null)
const saving = ref(false)

const imgSrc = ref('')
const scale = ref(1)
const offset = ref({ x: 0, y: 0 })

const natW = ref(0)
const natH = ref(0)
const stageSize = ref(0)

const cropSize = computed(() => Math.round(stageSize.value * 0.7))

// 图片在裁剪舞台中的变换：先位移后缩放
const imgStyle = computed(() => ({
  transform: `translate(${offset.value.x}px, ${offset.value.y}px) scale(${scale.value})`,
}))

// 预览圆（72px）与裁剪框（cropSize）的比例，将舞台状态等比映射到预览
const previewRatio = computed(() => (cropSize.value ? 72 / cropSize.value : 1))
const previewImgStyle = computed(() => ({
  transform: `translate(${offset.value.x * previewRatio.value}px, ${offset.value.y * previewRatio.value}px) scale(${scale.value * previewRatio.value})`,
}))

let dragging = false
let startX = 0
let startY = 0
let startOffset = { x: 0, y: 0 }

function pick() {
  fileRef.value?.click()
}

function onFileChange(e) {
  const file = e.target.files?.[0]
  if (!file) return
  if (file.size > 2 * 1024 * 1024) {
    ElMessage.warning('图片大小不能超过 2MB')
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    imgSrc.value = reader.result
    scale.value = 1
    offset.value = { x: 0, y: 0 }
  }
  reader.readAsDataURL(file)
  e.target.value = ''
}

function onImgLoad() {
  natW.value = imgRef.value.naturalWidth
  natH.value = imgRef.value.naturalHeight
  stageSize.value = stageRef.value.clientWidth
  resetToFit()
}

function resetToFit() {
  // 让图片最短边刚好覆盖裁剪框
  const s = Math.max(cropSize.value / natW.value, cropSize.value / natH.value)
  scale.value = s
  const dw = natW.value * s
  const dh = natH.value * s
  offset.value = {
    x: (stageSize.value - dw) / 2,
    y: (stageSize.value - dh) / 2,
  }
}

function onMouseDown(e) {
  if (!imgSrc.value) return
  dragging = true
  startX = e.clientX
  startY = e.clientY
  startOffset = { ...offset.value }
  e.preventDefault()
}

function onMouseMove(e) {
  if (!dragging) return
  offset.value = {
    x: startOffset.x + (e.clientX - startX),
    y: startOffset.y + (e.clientY - startY),
  }
}

function onMouseUp() {
  dragging = false
}

function clampOffset() {
  const dw = natW.value * scale.value
  const dh = natH.value * scale.value
  const left = (stageSize.value - cropSize.value) / 2
  // 允许一定拖出，但保留至少 10px 覆盖，避免完全拖空
  const minX = left - dw + 10
  const maxX = left + cropSize.value - 10
  offset.value.x = Math.min(Math.max(offset.value.x, minX), maxX)
  const minY = left - dh + 10
  const maxY = left + cropSize.value - 10
  offset.value.y = Math.min(Math.max(offset.value.y, minY), maxY)
}

onMounted(() => {
  const el = stageRef.value
  if (el) el.addEventListener('mousedown', onMouseDown)
  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)
})

async function confirm() {
  if (!imgSrc.value) return
  clampOffset()
  saving.value = true
  try {
    const blob = await cropToBlob()
    emit('confirm', blob)
  } catch (e) {
    ElMessage.error('裁剪失败：' + (e?.message || e))
  } finally {
    saving.value = false
  }
}

function cropToBlob() {
  return new Promise((resolve, reject) => {
    const img = imgRef.value
    if (!img) return reject(new Error('图片未加载'))
    const size = 256
    const canvas = document.createElement('canvas')
    canvas.width = size
    canvas.height = size
    const ctx = canvas.getContext('2d')
    const left = (stageSize.value - cropSize.value) / 2
    const sx = (left - offset.value.x) / scale.value
    const sy = (left - offset.value.y) / scale.value
    const sw = cropSize.value / scale.value
    ctx.drawImage(img, sx, sy, sw, sw, 0, 0, size, size)
    canvas.toBlob((blob) => {
      if (blob) resolve(blob)
      else reject(new Error('生成图片失败'))
    }, 'image/png')
  })
}

defineExpose({ pick })
</script>

<style scoped>
.cropper {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
}
.crop-stage {
  position: relative;
  width: 320px;
  height: 320px;
  background: #f0f0f0;
  border-radius: 8px;
  overflow: hidden;
  cursor: grab;
  user-select: none;
}
.crop-stage:active {
  cursor: grabbing;
}
.crop-img {
  position: absolute;
  top: 0;
  left: 0;
  transform-origin: 0 0;
}
.crop-mask {
  position: absolute;
  inset: 0;
  pointer-events: none;
}
.crop-frame {
  position: absolute;
  left: 50%;
  top: 50%;
  width: 70%;
  height: 70%;
  transform: translate(-50%, -50%);
  border: 2px solid #409eff;
  box-shadow: 0 0 0 9999px rgba(0, 0, 0, 0.45);
}
.crop-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #c0c4cc;
}
.crop-preview {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}
.preview-circle {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  background: #f0f0f0;
  overflow: hidden;
  border: 2px solid #dcdfe6;
}
.preview-img {
  width: auto;
  height: auto;
  transform-origin: 0 0;
}
.preview-tip {
  font-size: 12px;
  color: #909399;
}
.crop-controls {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.crop-actions {
  display: flex;
  justify-content: center;
  gap: 12px;
}
.hidden-input {
  display: none;
}
</style>

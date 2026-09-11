<template>
  <div class="cropper">
    <div
      ref="stageRef"
      class="crop-stage"
      @wheel.prevent="onWheel"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerUp"
    >
      <img
        v-if="imgSrc"
        ref="imgRef"
        :src="imgSrc"
        class="crop-img"
        :style="imgStyle"
        draggable="false"
        alt="待裁剪图片"
        @load="onImgLoad"
        @error="onImgError"
      />
      <div v-if="ready" class="crop-mask">
        <div class="crop-frame" />
      </div>
      <div v-if="decoding" class="stage-loading">
        <el-icon class="is-loading"><Loading /></el-icon>
        <span>正在读取图片…</span>
      </div>
      <div v-if="!imgSrc" class="crop-empty" @click="pick">
        <el-icon class="empty-icon"><Picture /></el-icon>
        <span>点击选择图片</span>
        <span class="empty-sub">支持 jpg / png / webp，手机原图也可直接选</span>
      </div>
    </div>

    <div class="crop-body">
      <div class="crop-preview">
        <canvas ref="previewRef" :width="OUTPUT_SIZE" :height="OUTPUT_SIZE" class="preview-circle" />
        <span class="preview-tip">头像预览</span>
      </div>

      <div class="crop-controls">
        <div class="zoom-row">
          <span class="zoom-label">缩放</span>
          <el-slider
            :model-value="zoom"
            :min="1"
            :max="4"
            :step="0.01"
            :disabled="!ready"
            class="zoom-slider"
            @input="applyZoom"
          />
        </div>
        <div class="crop-actions">
          <el-button @click="pick">选择图片</el-button>
          <el-button type="primary" :disabled="!ready" :loading="saving" @click="confirm">确定并上传</el-button>
        </div>
        <div class="crop-hint">拖动图片调整位置，滚轮或滑块缩放；上传的是 {{ OUTPUT_SIZE }}×{{ OUTPUT_SIZE }} 裁剪结果</div>
      </div>
    </div>

    <input ref="fileRef" type="file" accept="image/*" class="hidden-input" @change="onFileChange" />
  </div>
</template>

<script setup>
import { computed, onUnmounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'

// 裁剪交互约定（改动前请先读）：
// 1. 「选择图片」在任何状态下都可点击——首次还没有图片时恰恰最需要它。
// 2. 缩放用「相对适配值的倍率 zoom」而不是绝对倍率：fitScale 让图片短边刚好铺满裁剪框，
//    因此整张图始终可见，zoom 只能放大（1~4），不会出现「缩小到看不见」或「四周留白边」。
// 3. 预览直接用 canvas 画最终裁剪结果（圆形取景），不再复制一套 CSS 变换，避免预览与实际结果错位。
// 4. 上传的是裁剪后的 256px 小图，所以对「原图」只设很宽的防呆上限，不再拿 2MB 拦用户。

const emit = defineEmits(['confirm'])

const OUTPUT_SIZE = 256 // 裁剪输出边长（正方形）
const MAX_SOURCE_MB = 20 // 原图大小上限，仅防呆；裁剪后上传体积由后端 2MB 校验兜底
const CROP_RATIO = 0.7 // 裁剪框占舞台宽度的比例，与 .crop-frame 的 70% 对应

const stageRef = ref(null)
const imgRef = ref(null)
const fileRef = ref(null)
const previewRef = ref(null)

const imgSrc = ref('')
const decoding = ref(false)
const saving = ref(false)

const zoom = ref(1)
const offset = ref({ x: 0, y: 0 })

const natW = ref(0)
const natH = ref(0)
const stageSize = ref(0)

const cropSize = computed(() => Math.round(stageSize.value * CROP_RATIO))
const ready = computed(() => !!imgSrc.value && natW.value > 0 && natH.value > 0 && cropSize.value > 0)

// fitScale：图片短边刚好覆盖裁剪框；scale：最终显示倍率
const fitScale = computed(() => {
  if (!natW.value || !natH.value || !cropSize.value) return 1
  return Math.max(cropSize.value / natW.value, cropSize.value / natH.value)
})
const scale = computed(() => fitScale.value * zoom.value)

const imgStyle = computed(() => ({
  transform: `translate(${drawOffset.value.x}px, ${drawOffset.value.y}px) scale(${scale.value})`,
}))

// 显示与导出共用的唯一位置来源：即使某条路径改缩放/尺寸后忘了约束，
// 预览与上传结果也不可能分叉（以前就是导出前才 clamp，导致“框里空白、传上去却是另一张图”）。
const drawOffset = computed(() => {
  const o = offset.value
  if (!stageSize.value || !natW.value || !natH.value) return o
  const dw = natW.value * scale.value
  const dh = natH.value * scale.value
  const left = (stageSize.value - cropSize.value) / 2
  return {
    x: clamp(o.x, left + cropSize.value - dw, left),
    y: clamp(o.y, left + cropSize.value - dh, left),
  }
})

function clamp(v, min, max) {
  if (max < min) return (min + max) / 2
  return Math.min(Math.max(v, min), max)
}

// 裁剪框（舞台居中）在图片坐标系中的取样区域：源图上的左上角与边长
const cropRect = computed(() => {
  const left = (stageSize.value - cropSize.value) / 2
  const s = scale.value || 1
  return {
    sx: (left - drawOffset.value.x) / s,
    sy: (left - drawOffset.value.y) / s,
    ss: cropSize.value / s,
  }
})

function pick() {
  fileRef.value?.click()
}

function releaseObjectUrl() {
  if (imgSrc.value && imgSrc.value.startsWith('blob:')) {
    URL.revokeObjectURL(imgSrc.value)
  }
}

function onFileChange(e) {
  const input = e.target
  const file = input.files?.[0]
  input.value = '' // 无论后续分支如何都要清空，否则再选同一个文件不会触发 change
  if (!file) return
  if (!file.type.startsWith('image/')) {
    ElMessage.warning('只能选择图片文件')
    return
  }
  if (file.size > MAX_SOURCE_MB * 1024 * 1024) {
    ElMessage.warning(`图片过大（约 ${(file.size / 1024 / 1024).toFixed(1)}MB），请压缩后再选`)
    return
  }
  // 用 object URL 而不是 dataURL：不走 base64，大图不致于多占三分之一内存
  releaseObjectUrl()
  decoding.value = true
  imgSrc.value = URL.createObjectURL(file)
}

function onImgLoad() {
  const img = imgRef.value
  natW.value = img.naturalWidth
  natH.value = img.naturalHeight
  stageSize.value = stageRef.value?.clientWidth || 0
  zoom.value = 1
  if (Math.min(natW.value, natH.value) < 128) {
    ElMessage.warning('图片分辨率较低，头像可能模糊')
  }
  centerOffset()
  renderPreview()
  decoding.value = false
}

function onImgError() {
  releaseObjectUrl()
  imgSrc.value = ''
  natW.value = 0
  natH.value = 0
  decoding.value = false
  ElMessage.error('图片无法读取，请换一张（不支持 HEIC / TIFF 等格式）')
}

// 图片相对裁剪框居中；同时保证裁剪框被完全覆盖（不露白边）
function centerOffset() {
  const dw = natW.value * scale.value
  const dh = natH.value * scale.value
  offset.value = {
    x: (stageSize.value - dw) / 2,
    y: (stageSize.value - dh) / 2,
  }
  clampOffset()
}

function clampOffset() {
  const legal = drawOffset.value
  if (legal.x !== offset.value.x || legal.y !== offset.value.y) {
    offset.value = legal
  }
}

// 缩放时以裁剪框中心为锚点：不然一拉滑块画面就会跑向左上角，得重新拖动才能找回人脸
function keepFrameCenter(prevScale) {
  const c = stageSize.value / 2
  const s0 = prevScale || scale.value
  const s1 = scale.value
  if (!s0 || !s1 || s0 === s1) return
  const ux = (c - offset.value.x) / s0
  const uy = (c - offset.value.y) / s0
  offset.value = { x: c - ux * s1, y: c - uy * s1 }
  clampOffset()
}

// 缩放唯一入口：以裁剪框中心为锚点，并重新约束位置。
// 不用 watch(zoom)：避开滑块双向绑定与“新图载入时重置 zoom=1”互相干扰，
// 后者会把刚刚居中的画面又按旧图的锚点挪一次。
function applyZoom(next) {
  const clamped = clamp(Math.round(Number(next) * 100) / 100, 1, 4)
  if (clamped === zoom.value) return
  const prevScale = scale.value
  zoom.value = clamped
  keepFrameCenter(prevScale)
}

let dragging = false
let pointerId = null
let startX = 0
let startY = 0
let startOffset = { x: 0, y: 0 }

function onPointerDown(e) {
  if (!ready.value || (e.pointerType === 'mouse' && e.button !== 0)) return
  dragging = true
  pointerId = e.pointerId
  startX = e.clientX
  startY = e.clientY
  startOffset = { ...offset.value }
  // 抓住舞台后，后续 pointer 事件都回到本元素，不必再挂 window 监听
  stageRef.value?.setPointerCapture?.(e.pointerId)
  e.preventDefault()
}

function onPointerMove(e) {
  if (!dragging || e.pointerId !== pointerId) return
  offset.value = {
    x: startOffset.x + (e.clientX - startX),
    y: startOffset.y + (e.clientY - startY),
  }
  clampOffset()
}

function onPointerUp(e) {
  if (!dragging || (pointerId !== null && e.pointerId !== pointerId)) return
  const id = pointerId
  dragging = false
  pointerId = null
  const stage = stageRef.value
  if (stage && id !== null && stage.hasPointerCapture?.(id)) {
    stage.releasePointerCapture(id)
  }
}

function onWheel(e) {
  if (!ready.value) return
  applyZoom(zoom.value + (e.deltaY < 0 ? 0.1 : -0.1))
}

// 把当前取景画到指定 canvas（预览与上传共用同一份几何，杜绝「预览好看、上传跑偏」）
function drawCrop(canvas, size, circular) {
  const img = imgRef.value
  if (!canvas || !img || !size) return false
  canvas.width = size
  canvas.height = size
  const ctx = canvas.getContext('2d')
  if (!ctx) return false
  ctx.clearRect(0, 0, size, size)
  ctx.save()
  if (circular) {
    ctx.beginPath()
    ctx.arc(size / 2, size / 2, size / 2, 0, Math.PI * 2)
    ctx.clip()
  }
  const { sx, sy, ss } = cropRect.value
  ctx.drawImage(img, sx, sy, ss, ss, 0, 0, size, size)
  ctx.restore()
  return true
}

let previewQueued = false
function renderPreview() {
  if (!ready.value) {
    const canvas = previewRef.value
    if (canvas && canvas.width) {
      const ctx = canvas.getContext('2d')
      ctx?.clearRect(0, 0, canvas.width, canvas.height)
    }
    return
  }
  if (previewQueued) return
  previewQueued = true
  requestAnimationFrame(() => {
    previewQueued = false
    drawCrop(previewRef.value, OUTPUT_SIZE, true)
  })
}

watch([offset, zoom, imgSrc, stageSize], renderPreview, { immediate: true })

function toBlob() {
  return new Promise((resolve, reject) => {
    const canvas = document.createElement('canvas')
    if (!drawCrop(canvas, OUTPUT_SIZE, false)) {
      reject(new Error('图片未加载'))
      return
    }
    canvas.toBlob(
      (blob) => (blob ? resolve(blob) : reject(new Error('生成图片失败'))),
      'image/png',
    )
  })
}

async function confirm() {
  if (!ready.value) return
  clampOffset()
  saving.value = true
  try {
    const blob = await toBlob()
    emit('confirm', blob)
  } catch (e) {
    ElMessage.error('裁剪失败：' + (e?.message || e))
  } finally {
    saving.value = false
  }
}

onUnmounted(() => {
  releaseObjectUrl()
})

defineExpose({ pick })
</script>

<style scoped>
.cropper {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.crop-stage {
  position: relative;
  width: 320px;
  height: 320px;
  margin: 0 auto;
  background: #f0f0f0;
  border-radius: 8px;
  overflow: hidden;
  cursor: grab;
  user-select: none;
  touch-action: none; /* 触摸拖动时不让页面跟着滚 */
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
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: #909399;
  cursor: pointer;
}
.crop-empty:hover {
  color: #409eff;
  background: #eceff5;
}
.empty-icon {
  font-size: 34px;
}
.empty-sub {
  font-size: 12px;
  color: #c0c4cc;
}
.stage-loading {
  position: absolute;
  inset: 0;
  z-index: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: #606266;
  background: rgba(240, 240, 240, 0.9);
  font-size: 13px;
}
.crop-body {
  display: flex;
  align-items: flex-start;
  gap: 18px;
}
.crop-preview {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  flex: none;
}
.preview-circle {
  width: 72px;
  height: 72px;
  border-radius: 50%;
  background: #f0f0f0;
  border: 2px solid #dcdfe6;
}
.preview-tip {
  font-size: 12px;
  color: #909399;
}
.crop-controls {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.zoom-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.zoom-label {
  font-size: 13px;
  color: #606266;
  flex: none;
}
.zoom-slider {
  flex: 1;
}
.crop-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
.crop-hint {
  font-size: 12px;
  line-height: 1.5;
  color: #909399;
}
.hidden-input {
  display: none;
}
</style>

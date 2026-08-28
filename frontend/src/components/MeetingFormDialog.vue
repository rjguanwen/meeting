<template>
  <el-dialog
    v-model="visible"
    :title="title"
    width="520px"
    :close-on-click-modal="false"
    append-to-body
  >
    <el-form :model="form" label-width="90px">
      <el-form-item label="会议名称" required>
        <el-input v-model="form.title" placeholder="请输入会议名称" maxlength="100" />
      </el-form-item>
      <el-form-item label="会议地点">
        <el-select
          v-model="form.location"
          filterable
          allow-create
          default-first-option
          clearable
          placeholder="选择会议室或输入自定义地点"
          style="width: 100%"
        >
          <el-option v-for="r in rooms" :key="r.id" :label="roomLabel(r)" :value="r.name" />
        </el-select>
      </el-form-item>
      <el-form-item label="会议时间">
        <el-date-picker
          v-model="form.meeting_time"
          type="datetime"
          value-format="YYYY-MM-DD HH:mm:ss"
          placeholder="请选择会议时间"
          style="width: 100%"
        />
      </el-form-item>
      <el-form-item label="是否保密">
        <el-switch v-model="form.is_confidential" />
        <span class="conf-tip">{{ form.is_confidential ? '保密会议：仅参会组织负责人可查看' : '公开会议：材料向所有参会组织成员公开' }}</span>
      </el-form-item>
      <el-form-item label="参会组织">
        <el-select
          v-model="orgIds"
          multiple
          filterable
          :disabled="!allowOrgs"
          placeholder="选择参会组织（可多选）"
          style="width: 100%"
        >
          <el-option-group v-for="d in orgTree" :key="d.id" :label="d.name">
            <el-option :label="d.name" :value="d.id" />
            <el-option v-for="t in d.children || []" :key="t.id" :label="`↳ ${t.name}`" :value="t.id" />
          </el-option-group>
        </el-select>
        <div v-if="!allowOrgs" class="org-tip">仅筹备中的会议可以修改参会组织</div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="loading" @click="submit">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { orgApi, roomApi } from '../api'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  title: { type: String, default: '会议' },
  initial: { type: Object, default: () => ({}) },
  allowOrgs: { type: Boolean, default: true },
  loading: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue', 'submit'])

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})
const form = reactive({ title: '', location: '', meeting_time: '', is_confidential: false })
const orgIds = ref([])
const orgTree = ref([])
const rooms = ref([])

watch(
  () => props.modelValue,
  (v) => {
    if (!v) return
    form.title = props.initial.title || ''
    form.location = props.initial.location || ''
    form.meeting_time = props.initial.meeting_time || ''
    form.is_confidential = !!props.initial.is_confidential
    orgIds.value = props.initial.org_ids || []
    if (!orgTree.value.length) loadOrgs()
    if (!rooms.value.length) loadRooms()
  },
)

async function loadOrgs() {
  try {
    orgTree.value = await orgApi.tree()
  } catch {
    orgTree.value = []
  }
}

async function loadRooms() {
  try {
    rooms.value = await roomApi.list()
  } catch {
    rooms.value = []
  }
}

function roomLabel(r) {
  const extra = []
  if (r.location) extra.push(r.location)
  if (r.capacity) extra.push(`${r.capacity} 人`)
  if (!r.is_active) extra.push('停用')
  return extra.length ? `${r.name}（${extra.join('，')}）` : r.name
}

function submit() {
  if (!form.title.trim()) {
    ElMessage.warning('请输入会议名称')
    return
  }
  const payload = {
    title: form.title.trim(),
    location: form.location.trim(),
    is_confidential: form.is_confidential,
  }
  if (props.allowOrgs) payload.org_ids = orgIds.value
  if (form.meeting_time) payload.meeting_time = form.meeting_time
  emit('submit', payload)
}
</script>

<style scoped>
.org-tip {
  color: #909399;
  font-size: 12px;
  margin-top: 4px;
}
.conf-tip {
  color: #909399;
  font-size: 12px;
  margin-left: 10px;
}
</style>

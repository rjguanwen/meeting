<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>会议室管理</span>
        <el-button type="primary" :icon="Plus" @click="openCreate">新增会议室</el-button>
      </div>
    </template>

    <el-alert
      title="维护会议室后，创建/编辑会议时可在「会议地点」中直接选择，也可以自定义输入地点。"
      type="info"
      :closable="false"
      show-icon
      class="tip"
    />

    <el-table :data="rooms" border v-loading="loading">
      <el-table-column prop="name" label="会议室名称" min-width="160" />
      <el-table-column prop="location" label="所在位置" min-width="160">
        <template #default="{ row }">{{ row.location || '—' }}</template>
      </el-table-column>
      <el-table-column prop="capacity" label="容纳人数" width="100">
        <template #default="{ row }">{{ row.capacity || '—' }}</template>
      </el-table-column>
      <el-table-column prop="sort_order" label="排序" width="80" />
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.is_active ? 'success' : 'info'">{{ row.is_active ? '启用' : '停用' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="remark" label="备注" min-width="160">
        <template #default="{ row }">{{ row.remark || '—' }}</template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="remove(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-empty v-if="!loading && !rooms.length" description="暂无会议室，请先新增" />

    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑会议室' : '新增会议室'" width="480px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="会议室名称" maxlength="128" />
        </el-form-item>
        <el-form-item label="所在位置">
          <el-input v-model="form.location" placeholder="如：A 栋 3 楼" maxlength="255" />
        </el-form-item>
        <el-form-item label="容纳人数">
          <el-input-number v-model="form.capacity" :min="0" :max="10000" controls-position="right" style="width: 100%" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort_order" :min="0" :max="9999" controls-position="right" style="width: 100%" />
        </el-form-item>
        <el-form-item label="状态">
          <el-switch v-model="form.is_active" active-text="启用" inactive-text="停用" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" type="textarea" :rows="2" maxlength="512" placeholder="选填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { roomApi } from '../api'

const rooms = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const saving = ref(false)
const form = reactive({ id: null, name: '', location: '', capacity: 0, sort_order: 0, is_active: true, remark: '' })

async function load() {
  loading.value = true
  try {
    rooms.value = await roomApi.list()
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(form, { id: null, name: '', location: '', capacity: 0, sort_order: 0, is_active: true, remark: '' })
  dialogVisible.value = true
}

function openEdit(row) {
  Object.assign(form, {
    id: row.id,
    name: row.name,
    location: row.location || '',
    capacity: row.capacity || 0,
    sort_order: row.sort_order || 0,
    is_active: row.is_active,
    remark: row.remark || '',
  })
  dialogVisible.value = true
}

async function save() {
  if (!form.name.trim()) {
    ElMessage.warning('请输入会议室名称')
    return
  }
  saving.value = true
  try {
    const payload = { ...form }
    if (form.id) {
      await roomApi.update(form.id, payload)
    } else {
      await roomApi.create(payload)
    }
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

function remove(row) {
  ElMessageBox.confirm(`确定删除会议室「${row.name}」吗？`, '提示', { type: 'warning' })
    .then(async () => {
      await roomApi.remove(row.id)
      ElMessage.success('已删除')
      await load()
    })
    .catch(() => {})
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
</style>

<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>组织管理（部门 / 小组）</span>
        <div>
          <el-button type="primary" :icon="Plus" @click="openDialog('dept')">新增部门</el-button>
          <el-button :icon="Plus" @click="openDialog('team')">新增小组</el-button>
        </div>
      </div>
    </template>

    <el-alert
      title="组织负责人登录对应小组账号后，即可为所属小组录入会议汇报事项。一个部门/小组可设置多个负责人账号。"
      type="info"
      :closable="false"
      show-icon
      class="tip"
    />

    <el-table :data="tree" row-key="id" :tree-props="{ children: 'children' }" border>
      <el-table-column prop="name" label="组织名称" min-width="200">
        <template #default="{ row }">
          <el-tag v-if="row.type === 'team'" size="small" type="warning">小组</el-tag>
          <el-tag v-else size="small" type="primary">部门</el-tag>
          <span class="org-name">{{ row.name }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="sort_order" label="排序" width="80" />
      <el-table-column label="操作" width="200">
        <template #default="{ row }">
          <el-button size="small" @click="editOrg(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="removeOrg(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-empty v-if="!tree.length" description="暂无组织，请先新增部门" />

    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑组织' : (form.type === 'dept' ? '新增部门' : '新增小组')" width="440px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="组织名称" required>
          <el-input v-model="form.name" placeholder="请输入名称" />
        </el-form-item>
        <el-form-item v-if="form.type === 'team'" label="所属部门" required>
          <el-select v-model="form.parent_id" placeholder="请选择所属部门" style="width: 100%">
            <el-option v-for="d in depts" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort_order" :min="0" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveOrg">保存</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { orgApi } from '../api'

const tree = ref([])
const dialogVisible = ref(false)
const saving = ref(false)
const form = reactive({ id: null, name: '', type: 'dept', parent_id: null, sort_order: 0 })

const depts = computed(() => tree.value)

async function loadTree() {
  tree.value = await orgApi.tree()
}

function openDialog(type) {
  Object.assign(form, { id: null, name: '', type, parent_id: null, sort_order: 0 })
  dialogVisible.value = true
}

function editOrg(row) {
  Object.assign(form, { id: row.id, name: row.name, type: row.type, parent_id: row.parent_id, sort_order: row.sort_order })
  dialogVisible.value = true
}

async function saveOrg() {
  if (!form.name.trim()) {
    ElMessage.warning('请输入组织名称')
    return
  }
  if (form.type === 'team' && !form.parent_id) {
    ElMessage.warning('请选择所属部门')
    return
  }
  saving.value = true
  try {
    if (form.id) {
      await orgApi.update(form.id, form)
    } else {
      await orgApi.create(form)
    }
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await loadTree()
  } finally {
    saving.value = false
  }
}

function removeOrg(row) {
  ElMessageBox.confirm(`确定删除组织「${row.name}」吗？`, '提示', { type: 'warning' })
    .then(async () => {
      await orgApi.remove(row.id)
      ElMessage.success('已删除')
      await loadTree()
    })
    .catch(() => {})
}

onMounted(loadTree)
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
.org-name {
  margin-left: 6px;
}
</style>

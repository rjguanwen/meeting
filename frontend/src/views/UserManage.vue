<template>
  <el-card shadow="never">
    <template #header>
      <div class="card-header">
        <span>账号管理</span>
        <el-button type="primary" :icon="Plus" @click="openCreate">新增账号</el-button>
      </div>
    </template>

    <el-alert
      title="每个部门 / 小组都可创建组织负责人账号，负责人登录后即可为所属组织录入会议汇报事项。"
      type="info"
      :closable="false"
      show-icon
      class="tip"
    />

    <el-table :data="users" border>
      <el-table-column prop="username" label="用户名" width="150" />
      <el-table-column prop="name" label="姓名" width="130" />
      <el-table-column label="角色" width="130">
        <template #default="{ row }">
          <el-tag :type="row.role === 'admin' ? 'danger' : 'success'">
            {{ row.role === 'admin' ? '管理员' : '组织负责人' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="所属组织" min-width="160">
        <template #default="{ row }">{{ row.org?.name || '—' }}</template>
      </el-table-column>
      <el-table-column label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.is_active ? 'success' : 'info'">{{ row.is_active ? '启用' : '停用' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="220">
        <template #default="{ row }">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button size="small" :type="row.is_active ? 'warning' : 'success'" @click="toggleActive(row)">
            {{ row.is_active ? '停用' : '启用' }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-empty v-if="!users.length" description="暂无账号" />

    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑账号' : '新增账号'" width="460px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="用户名" required>
          <el-input v-model="form.username" :disabled="!!form.id" placeholder="登录账号" />
        </el-form-item>
        <el-form-item label="姓名">
          <el-input v-model="form.name" placeholder="负责人姓名" />
        </el-form-item>
        <el-form-item label="密码" :required="!form.id">
          <el-input v-model="form.password" type="password" show-password :placeholder="form.id ? '留空则不修改' : '至少 6 位'" />
        </el-form-item>
        <el-form-item label="角色">
          <el-radio-group v-model="form.role">
            <el-radio value="leader">组织负责人</el-radio>
            <el-radio value="admin">管理员</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.role === 'leader'" label="所属组织" required>
          <el-tree-select
            v-model="form.org_id"
            :data="tree"
            :props="{ label: 'name', value: 'id', children: 'children' }"
            node-key="id"
            check-strictly
            placeholder="选择部门或小组"
            style="width: 100%"
          />
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
import { userApi, orgApi } from '../api'

const users = ref([])
const tree = ref([])
const dialogVisible = ref(false)
const saving = ref(false)
const form = reactive({ id: null, username: '', name: '', password: '', role: 'leader', org_id: null })

async function load() {
  users.value = await userApi.list()
  tree.value = await orgApi.tree()
}

function openCreate() {
  Object.assign(form, { id: null, username: '', name: '', password: '', role: 'leader', org_id: null })
  dialogVisible.value = true
}

function openEdit(row) {
  Object.assign(form, { id: row.id, username: row.username, name: row.name, password: '', role: row.role, org_id: row.org_id })
  dialogVisible.value = true
}

async function save() {
  if (!form.username.trim()) {
    ElMessage.warning('请输入用户名')
    return
  }
  if (!form.id && form.password.length < 6) {
    ElMessage.warning('密码至少 6 位')
    return
  }
  if (form.role === 'leader' && !form.org_id) {
    ElMessage.warning('请选择所属组织')
    return
  }
  saving.value = true
  try {
    const payload = { ...form }
    if (!payload.password) delete payload.password
    if (form.id) {
      await userApi.update(form.id, payload)
    } else {
      await userApi.create(payload)
    }
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await load()
  } finally {
    saving.value = false
  }
}

function toggleActive(row) {
  ElMessageBox.confirm(`确定${row.is_active ? '停用' : '启用'}账号「${row.username}」吗？`, '提示', { type: 'warning' })
    .then(async () => {
      await userApi.update(row.id, { is_active: !row.is_active })
      ElMessage.success('操作成功')
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

<template>
  <div class="profile">
    <el-card shadow="never" class="p-card">
      <template #header>个人资料</template>
      <div class="avatar-row">
        <div class="avatar-box">
          <el-avatar :size="80" :src="avatarUrl(auth.user?.avatar)" class="avatar">
            {{ avatarText }}
          </el-avatar>
          <el-button size="small" class="avatar-btn" @click="cropperVisible = true">更换头像</el-button>
        </div>
        <div class="profile-basic">
          <el-form label-width="80px" style="max-width: 360px">
            <el-form-item label="用户名">
              <el-input :model-value="auth.user?.username" disabled />
            </el-form-item>
            <el-form-item label="昵称">
              <el-input v-model="profileForm.name" placeholder="请输入昵称" />
            </el-form-item>
            <el-form-item label="角色">
              <el-tag :type="auth.isAdmin ? 'danger' : 'success'">{{ roleText }}</el-tag>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="profileSaving" @click="saveProfile">保存昵称</el-button>
            </el-form-item>
          </el-form>
        </div>
      </div>
    </el-card>

    <el-card shadow="never" class="p-card">
      <template #header>修改密码</template>
      <el-form label-width="90px" style="max-width: 420px">
        <el-form-item label="原密码">
          <el-input v-model="pwdForm.old_password" type="password" show-password placeholder="请输入原密码" />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="pwdForm.new_password" type="password" show-password placeholder="至少 6 位" />
        </el-form-item>
        <el-form-item label="确认新密码">
          <el-input v-model="pwdForm.confirm_password" type="password" show-password placeholder="再次输入新密码" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="pwdSaving" @click="savePassword">修改密码</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card shadow="never" class="p-card">
      <template #header>账号安全</template>
      <el-alert
        title="设置密码提示词可在忘记密码时提醒自己；设置安全问答后，可通过回答问题找回密码。"
        type="info"
        :closable="false"
        show-icon
        class="sec-tip"
      />
      <el-form label-width="100px" style="max-width: 480px">
        <el-form-item label="密码提示词">
          <el-input
            v-model="secForm.password_hint"
            placeholder="例如：我的工号 + 手机尾号（仅自己可见，用于提醒）"
          />
        </el-form-item>
        <el-form-item label="安全问题">
          <div class="question-row">
            <el-select v-model="secForm.question_mode" placeholder="选择预设问题" style="width: 180px">
              <el-option v-for="q in presetQuestions" :key="q" :label="q" :value="q" />
              <el-option label="自定义问题" value="__custom__" />
            </el-select>
            <el-input
              v-if="secForm.question_mode === '__custom__'"
              v-model="secForm.custom_question"
              placeholder="请输入自定义问题"
            />
          </div>
        </el-form-item>
        <el-form-item label="安全答案">
          <el-input v-model="secForm.security_answer" placeholder="安全问题的答案（留空则清空安全问答）" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="secSaving" @click="saveSecurity">保存安全设置</el-button>
          <el-button v-if="auth.user?.security_question" @click="clearSecurity">清空安全问答</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 头像裁剪弹窗 -->
    <el-dialog v-model="cropperVisible" title="更换头像" width="420px" destroy-on-close>
      <AvatarCropper ref="cropperRef" @confirm="onCropped" />
      <template #footer>
        <el-button @click="cropperVisible = false">取消</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { authApi, avatarUrl } from '../api'
import { useAuthStore } from '../stores/auth'
import AvatarCropper from '../components/AvatarCropper.vue'

const auth = useAuthStore()

const presetQuestions = [
  '我母亲的姓名是？',
  '我第一所学校是？',
  '我小时候最好的朋友是？',
  '我最喜欢的食物是？',
  '我第一份工作的公司是？',
]

const avatarText = computed(() => (auth.user?.name || auth.user?.username || '?').charAt(0))
const roleText = computed(() => {
  const m = { admin: '管理员', dept_leader: '部门负责人', team_leader: '小组负责人', member: '组织成员' }
  return m[auth.user?.role] || '组织用户'
})

// 个人资料
const profileForm = reactive({ name: '' })
const profileSaving = ref(false)
profileForm.name = auth.user?.name || ''

// 密码
const pwdForm = reactive({ old_password: '', new_password: '', confirm_password: '' })
const pwdSaving = ref(false)

// 安全设置
const secForm = reactive({
  password_hint: '',
  question_mode: '',
  custom_question: '',
  security_answer: '',
})
const secSaving = ref(false)

// 头像
const cropperVisible = ref(false)
const cropperRef = ref(null)

// 初始化安全设置（从 auth.user 读，但 user 可能不含 security 字段，用 me 刷新）
function initSecurity() {
  secForm.password_hint = auth.user?.password_hint || ''
  const q = auth.user?.security_question || ''
  if (q) {
    if (presetQuestions.includes(q)) {
      secForm.question_mode = q
      secForm.custom_question = ''
    } else {
      secForm.question_mode = '__custom__'
      secForm.custom_question = q
    }
  } else {
    secForm.question_mode = ''
    secForm.custom_question = ''
  }
}

async function refreshMe() {
  await auth.fetchMe()
  initSecurity()
}

async function saveProfile() {
  if (!profileForm.name.trim()) {
    ElMessage.warning('请输入昵称')
    return
  }
  profileSaving.value = true
  try {
    await authApi.updateProfile({ name: profileForm.name.trim() })
    ElMessage.success('昵称已保存')
    await refreshMe()
  } finally {
    profileSaving.value = false
  }
}

async function savePassword() {
  if (!pwdForm.old_password) {
    ElMessage.warning('请输入原密码')
    return
  }
  if (pwdForm.new_password.length < 6) {
    ElMessage.warning('新密码至少 6 位')
    return
  }
  if (pwdForm.new_password !== pwdForm.confirm_password) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  pwdSaving.value = true
  try {
    await authApi.changePassword({ old_password: pwdForm.old_password, new_password: pwdForm.new_password })
    ElMessage.success('密码修改成功，下次登录请使用新密码')
    Object.assign(pwdForm, { old_password: '', new_password: '', confirm_password: '' })
  } finally {
    pwdSaving.value = false
  }
}

function composedQuestion() {
  if (secForm.question_mode === '__custom__') {
    return secForm.custom_question.trim()
  }
  return secForm.question_mode
}

async function saveSecurity() {
  const question = composedQuestion()
  if (secForm.security_answer && !question) {
    ElMessage.warning('填写安全答案前请先选择或填写安全问题')
    return
  }
  secSaving.value = true
  try {
    await authApi.updateSecurity({
      password_hint: secForm.password_hint,
      security_question: question,
      security_answer: secForm.security_answer,
    })
    ElMessage.success('安全设置已保存')
    secForm.security_answer = ''
    await refreshMe()
  } finally {
    secSaving.value = false
  }
}

async function clearSecurity() {
  secSaving.value = true
  try {
    await authApi.updateSecurity({
      password_hint: secForm.password_hint,
      security_question: '',
      security_answer: '',
    })
    ElMessage.success('已清空安全问答')
    secForm.question_mode = ''
    secForm.custom_question = ''
    await refreshMe()
  } finally {
    secSaving.value = false
  }
}

async function onCropped(blob) {
  const file = new File([blob], 'avatar.png', { type: 'image/png' })
  try {
    await authApi.uploadAvatar(file)
    ElMessage.success('头像已更新')
    cropperVisible.value = false
    await refreshMe()
  } catch {
    // 错误已由拦截器提示
  }
}

initSecurity()
</script>

<style scoped>
.profile {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 860px;
}
.p-card :deep(.el-card__header) {
  font-weight: 600;
}
.avatar-row {
  display: flex;
  gap: 32px;
  align-items: flex-start;
}
.avatar-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
}
.avatar {
  background: #409eff;
  font-size: 28px;
}
.question-row {
  display: flex;
  gap: 12px;
  width: 100%;
  align-items: center;
}
.sec-tip {
  margin-bottom: 14px;
}
</style>

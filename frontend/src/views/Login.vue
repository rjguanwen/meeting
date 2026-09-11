<template>
  <div class="login-page">
    <el-card class="login-card">
      <div class="login-title">
        <el-icon :size="32" color="#409eff"><Monitor /></el-icon>
        <h2>技术部门会议系统</h2>
        <p>汇报录入 · 会议展示 · 纪要生成</p>
      </div>
      <el-form :model="form" :rules="rules" ref="formRef" size="large" @keyup.enter="onLogin">
        <el-form-item prop="username">
          <el-input v-model="form.username" placeholder="用户名" :prefix-icon="User" />
        </el-form-item>
        <el-form-item prop="password">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="密码"
            show-password
            :prefix-icon="Lock"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" class="login-btn" :loading="loading" @click="onLogin">
            登 录
          </el-button>
        </el-form-item>
        <div class="login-links">
          <el-link type="primary" :underline="false" @click="openForgot">忘记密码？</el-link>
        </div>
      </el-form>
    </el-card>

    <!-- 找回密码弹窗 -->
    <el-dialog v-model="forgotVisible" title="找回密码" width="420px">
      <!-- 第一步：输入用户名 -->
      <el-form v-if="forgotStep === 1" label-width="90px">
        <el-form-item label="用户名">
          <el-input v-model="forgotForm.username" placeholder="请输入登录用户名" />
        </el-form-item>
      </el-form>

      <!-- 第二步：回答安全问题 -->
      <div v-else-if="forgotStep === 2">
        <el-alert
          v-if="forgotHint.password_hint"
          :title="'密码提示：' + forgotHint.password_hint"
          type="info"
          :closable="false"
          show-icon
          class="forgot-hint"
        />
        <el-form label-width="90px">
          <el-form-item label="安全问题">
            <span class="sec-question">{{ forgotHint.security_question || '该账号未设置安全问题，无法自助找回，请联系管理员重置' }}</span>
          </el-form-item>
          <el-form-item v-if="forgotHint.security_question" label="安全答案">
            <el-input v-model="forgotForm.security_answer" placeholder="请输入安全答案" />
          </el-form-item>
          <el-form-item v-if="forgotHint.security_question" label="新密码">
            <el-input v-model="forgotForm.new_password" type="password" show-password placeholder="至少 6 位" />
          </el-form-item>
        </el-form>
      </div>

      <template #footer>
        <template v-if="forgotStep === 1">
          <el-button @click="forgotVisible = false">取消</el-button>
          <el-button type="primary" :loading="forgotLoading" @click="stepQueryQuestion">下一步</el-button>
        </template>
        <template v-else>
          <el-button @click="forgotStep = 1">上一步</el-button>
          <el-button v-if="forgotHint.security_question" type="primary" :loading="forgotLoading" @click="doReset">重置密码</el-button>
        </template>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { User, Lock } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '../stores/auth'
import { authApi } from '../api'

const formRef = ref()
const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const loading = ref(false)

const form = reactive({ username: '', password: '' })
const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

async function onLogin() {
  await formRef.value.validate().catch(() => Promise.reject())
  loading.value = true
  try {
    await auth.login(form.username, form.password)
    ElMessage.success('登录成功')
    router.push(route.query.redirect || '/dashboard')
  } finally {
    loading.value = false
  }
}

// 找回密码
const forgotVisible = ref(false)
const forgotStep = ref(1)
const forgotLoading = ref(false)
const forgotForm = reactive({ username: '', security_answer: '', new_password: '' })
const forgotHint = reactive({ password_hint: '', security_question: '' })

function openForgot() {
  forgotStep.value = 1
  forgotVisible.value = true
  Object.assign(forgotForm, { username: '', security_answer: '', new_password: '' })
  Object.assign(forgotHint, { password_hint: '', security_question: '' })
}

async function stepQueryQuestion() {
  if (!forgotForm.username.trim()) {
    ElMessage.warning('请输入用户名')
    return
  }
  forgotLoading.value = true
  try {
    const data = await authApi.forgotQuestion(forgotForm.username.trim())
    forgotHint.password_hint = data.password_hint
    forgotHint.security_question = data.security_question
    forgotStep.value = 2
  } finally {
    forgotLoading.value = false
  }
}

async function doReset() {
  if (!forgotForm.security_answer) {
    ElMessage.warning('请输入安全答案')
    return
  }
  if (forgotForm.new_password.length < 6) {
    ElMessage.warning('新密码至少 6 位')
    return
  }
  forgotLoading.value = true
  try {
    await authApi.forgotReset({
      username: forgotForm.username.trim(),
      security_answer: forgotForm.security_answer,
      new_password: forgotForm.new_password,
    })
    ElMessage.success('密码已重置，请使用新密码登录')
    forgotVisible.value = false
  } finally {
    forgotLoading.value = false
  }
}
</script>

<style scoped>
.login-page {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #1f3b73 0%, #2b5c8f 50%, #3a7bd5 100%);
}
.login-card {
  width: 400px;
  border-radius: 12px;
  padding: 12px 8px;
}
.login-title {
  text-align: center;
  margin-bottom: 24px;
}
.login-title h2 {
  margin: 8px 0 4px;
  color: #303133;
}
.login-title p {
  color: #909399;
  font-size: 13px;
}
.login-btn {
  width: 100%;
}
.login-links {
  text-align: right;
}
.forgot-hint {
  margin-bottom: 14px;
}
.sec-question {
  color: #303133;
  line-height: 1.6;
}
</style>

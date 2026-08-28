import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'

const api = axios.create({
  baseURL: '/api',
  timeout: 15000,
})

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    const status = error.response?.status
    const detail = error.response?.data?.detail
    if (status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
      if (router.currentRoute.value.path !== '/login') {
        router.push('/login')
      }
    }
    const msg = typeof detail === 'string' ? detail : error.message || '请求失败'
    ElMessage.error(msg)
    return Promise.reject(error)
  },
)

export default api

// ===== 认证 =====
export const authApi = {
  login: (username, password) => {
    const form = new URLSearchParams()
    form.append('username', username)
    form.append('password', password)
    return api.post('/auth/login', form)
  },
  me: () => api.get('/auth/me'),
  changePassword: (data) => api.patch('/auth/password', data),
}

// ===== 组织 =====
export const orgApi = {
  tree: () => api.get('/orgs/tree'),
  create: (data) => api.post('/orgs', data),
  update: (id, data) => api.patch(`/orgs/${id}`, data),
  remove: (id) => api.delete(`/orgs/${id}`),
}

// ===== 用户 =====
export const userApi = {
  list: () => api.get('/users'),
  create: (data) => api.post('/users', data),
  update: (id, data) => api.patch(`/users/${id}`, data),
}

// ===== 会议室 =====
export const roomApi = {
  list: () => api.get('/rooms'),
  create: (data) => api.post('/rooms', data),
  update: (id, data) => api.patch(`/rooms/${id}`, data),
  remove: (id) => api.delete(`/rooms/${id}`),
}

// ===== 会议 =====
export const meetingApi = {
  list: (params) => api.get('/meetings', { params }),
  get: (id) => api.get(`/meetings/${id}`),
  create: (data) => api.post('/meetings', data),
  update: (id, data) => api.patch(`/meetings/${id}`, data),
  remove: (id) => api.delete(`/meetings/${id}`),
  setOrgs: (id, orgIds) => api.post(`/meetings/${id}/orgs`, { org_ids: orgIds }),
  material: (id) => api.post(`/meetings/${id}/material`), // 生成（开始后仅一次）
  materialView: (id) => api.get(`/meetings/${id}/material`), // 只读查看
  start: (id) => api.post(`/meetings/${id}/start`),
  finish: (id) => api.post(`/meetings/${id}/finish`),
  archive: (id) => api.post(`/meetings/${id}/archive`),
}

// ===== 汇报事项 =====
export const itemApi = {
  list: (meetingId) => api.get(`/meetings/${meetingId}/items`),
  create: (meetingId, data) => api.post(`/meetings/${meetingId}/items`, data),
  update: (id, data) => api.patch(`/items/${id}`, data),
  remove: (id) => api.delete(`/items/${id}`),
}

// ===== 汇报事项附件 =====
export const attachmentApi = {
  list: (itemId) => api.get(`/items/${itemId}/attachments`),
  upload: (itemId, file, onUploadProgress) => {
    const form = new FormData()
    form.append('file', file)
    return api.post(`/items/${itemId}/attachments`, form, { onUploadProgress })
  },
  remove: (id) => api.delete(`/attachments/${id}`),
  fileUrl: (id) => `/api/attachments/${id}/file`, // 需带 token 访问，用 fetchFileBlob
}

// 获取附件 blob（带认证），供前端预览/下载
export async function fetchFileBlob(attId) {
  const blob = await api.get(`/attachments/${attId}/file`, { responseType: 'blob' })
  return blob
}

// ===== 讨论结论 / 新任务 =====
export const conclusionApi = {
  list: (meetingId) => api.get(`/meetings/${meetingId}/conclusions`),
  create: (meetingId, data) => api.post(`/meetings/${meetingId}/conclusions`, data),
  update: (id, data) => api.patch(`/conclusions/${id}`, data),
  remove: (id) => api.delete(`/conclusions/${id}`),
}

// ===== 会议纪要 =====
export const minutesApi = {
  get: (meetingId) => api.get(`/meetings/${meetingId}/minutes`),
  generate: (meetingId) => api.post(`/meetings/${meetingId}/minutes`),
  update: (meetingId, content) => api.patch(`/meetings/${meetingId}/minutes`, { content }),
}

// ===== 操作日志 =====
export const logApi = {
  list: (params) => api.get('/logs', { params }),
}

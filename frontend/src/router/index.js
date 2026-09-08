import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/login',
    name: 'login',
    component: () => import('../views/Login.vue'),
    meta: { public: true },
  },
  {
    path: '/',
    component: () => import('../layout/MainLayout.vue'),
    redirect: '/dashboard',
    children: [
      { path: 'dashboard', name: 'dashboard', component: () => import('../views/Dashboard.vue'), meta: { title: '首页' } },
      { path: 'orgs', name: 'orgs', component: () => import('../views/OrgManage.vue'), meta: { title: '组织管理', admin: true } },
      { path: 'users', name: 'users', component: () => import('../views/UserManage.vue'), meta: { title: '账号管理', admin: true } },
      { path: 'rooms', name: 'rooms', component: () => import('../views/RoomManage.vue'), meta: { title: '会议室管理', admin: true } },
      { path: 'logs', name: 'logs', component: () => import('../views/LogManage.vue'), meta: { title: '操作日志', admin: true } },
      { path: 'meetings', name: 'meetings', component: () => import('../views/MeetingList.vue'), meta: { title: '会议管理' } },
      { path: 'meetings/:id', name: 'meeting-detail', component: () => import('../views/MeetingDetail.vue'), meta: { title: '会议详情' } },
      { path: 'entry/:id', name: 'report-entry', component: () => import('../views/ReportEntry.vue'), meta: { title: '汇报事项录入' } },
      { path: 'guide', name: 'guide', component: () => import('../views/Guide.vue'), meta: { title: '使用手册' } },
    ],
  },
  {
    path: '/show/:id',
    name: 'meeting-show',
    component: () => import('../views/MeetingShow.vue'),
    meta: { title: '会议展示' },
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const token = localStorage.getItem('token')
  if (!to.meta.public && !token) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.meta.admin) {
    const user = JSON.parse(localStorage.getItem('user') || 'null')
    if (!user || user.role !== 'admin') {
      return { name: 'dashboard' }
    }
  }
  if (to.name === 'login' && token) {
    return { name: 'dashboard' }
  }
  return true
})

export default router

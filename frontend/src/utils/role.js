// 角色常量与文案映射
export const ROLE_TEXT = {
  admin: '管理员',
  dept_leader: '部门负责人',
  team_leader: '小组负责人',
  member: '组织成员',
}

export const ROLE_TAG_TYPE = {
  admin: 'danger',
  dept_leader: 'primary',
  team_leader: 'warning',
  member: 'success',
}

export function roleText(role) {
  return ROLE_TEXT[role] || role || '—'
}

export function roleTagType(role) {
  return ROLE_TAG_TYPE[role] || 'info'
}

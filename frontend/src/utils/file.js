import { fetchFileBlob } from '../api'

// 附件类型判断
export function isImage(att) {
  return /^image\//.test(att.mime_type || '')
}
export function isVideo(att) {
  return /^video\//.test(att.mime_type || '')
}
export function isPdf(att) {
  return (att.mime_type || '').includes('pdf') || /\.pdf$/i.test(att.file_name || '')
}
export function isAudio(att) {
  return /^audio\//.test(att.mime_type || '')
}

// 可在线预览的类型（图片/视频/pdf/音频）
export function isPreviewable(att) {
  return isImage(att) || isVideo(att) || isPdf(att) || isAudio(att)
}

// 格式化文件大小
export function fmtSize(bytes) {
  if (!bytes && bytes !== 0) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

// 从后端拉取附件并生成可预览的 blob URL（带 token 认证）
const urlCache = new Map()

export async function getFileUrl(attId) {
  if (urlCache.has(attId)) return urlCache.get(attId)
  const blob = await fetchFileBlob(attId)
  const url = URL.createObjectURL(blob)
  urlCache.set(attId, url)
  return url
}

// 下载附件
export async function downloadFile(att) {
  const url = await getFileUrl(att.id)
  const a = document.createElement('a')
  a.href = url
  a.download = att.file_name
  a.click()
}

// 新窗口打开（PDF 等）
export async function openFileInNewTab(att) {
  const url = await getFileUrl(att.id)
  window.open(url, '_blank')
}

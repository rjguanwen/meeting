import { marked } from 'marked'

// gfm+breaks：兼容历史多行纯文本记录；忽略用户内容中的原始 HTML（防 XSS）
marked.use({
  gfm: true,
  breaks: true,
  renderer: {
    html: () => '',
  },
})

// renderMarkdown 将 Markdown 渲染为 HTML（用于 v-html 展示）。
export function renderMarkdown(src) {
  if (!src) return ''
  let html = String(marked.parse(src))
  // 兜底清理：事件属性与 javascript: 伪协议链接
  html = html
    .replace(/\son[a-z]+\s*=\s*"[^"]*"/gi, '')
    .replace(/href\s*=\s*"javascript:[^"]*"/gi, 'href="#"')
  return html
}

// stripMarkdown 去除常见 Markdown 标记，得到纯文本（用于列表/提示等轻量展示）。
export function stripMarkdown(src) {
  if (!src) return ''
  return src
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/^\s*[-*+]\s+/gm, '')
    .replace(/^\s*\d+[.)]\s+/gm, '')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/^\s*>\s?/gm, '')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/__([^_]+)__/g, '$1')
    .replace(/\*([^*]+)\*/g, '$1')
    .replace(/~~([^~]+)~~/g, '$1')
    .replace(/^[-=]{3,}\s*$/gm, '')
    .replace(/[|:]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

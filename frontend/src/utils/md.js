import { marked } from 'marked'

// 链接允许的外部协议；其余（javascript: / vbscript: / data: 等）一律降级为纯文本。
const ALLOWED_SCHEMES = /^(?:https?|mailto|tel|ftp):/i
// 图片额外允许内联 base64：历史内容可能粘贴过截图
const INLINE_IMAGE = /^data:image\/(?:png|jpe?g|gif|webp|bmp);base64,/i
// 协议探测前需剔除控制字符，否则 `java\tscript:` 一类写法可绕过判定
const CONTROL_CHARS = /[\u0000-\u0020\u007f]/g

// 仅转义能破坏属性 / 提前闭合标签的字符；& 不转义，以与 marked 默认输出一致（避免 ?a=1&amp;b=2 被二次编码）。
function escapeAttr(value) {
  return String(value ?? '')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

// safeUrl 返回可写入属性的 URL；不可信时返回空串。未带协议的相对/站内路径直接放行。
function safeUrl(href, ...extraRe) {
  const raw = String(href ?? '').trim()
  const probe = raw.replace(CONTROL_CHARS, '')
  if (!/^[a-z][a-z0-9+.-]*:/i.test(probe)) return raw
  if (ALLOWED_SCHEMES.test(probe) || extraRe.some((re) => re.test(probe))) return raw
  return ''
}

// gfm+breaks：兼容历史多行纯文本记录；忽略用户内容中的原始 HTML（防 XSS）
marked.use({
  gfm: true,
  breaks: true,
  renderer: {
    html: () => '',
    // marked 自身不过滤 javascript: 伪协议，因此在渲染阶段做协议白名单（比事后正则覆盖更全）
    link({ href, title, tokens }) {
      const text = this.parser.parseInline(tokens)
      const url = safeUrl(href)
      if (!url) return text
      const titleAttr = title ? ` title="${escapeAttr(title)}"` : ''
      return `<a href="${escapeAttr(url)}"${titleAttr}>${text}</a>`
    },
    image({ href, title, text }) {
      const url = safeUrl(href, INLINE_IMAGE)
      if (!url) return ''
      const titleAttr = title ? ` title="${escapeAttr(title)}"` : ''
      return `<img src="${escapeAttr(url)}" alt="${escapeAttr(text)}"${titleAttr}>`
    },
  },
})

// renderMarkdown 将 Markdown 渲染为 HTML（用于 v-html 展示）。
export function renderMarkdown(src) {
  if (!src) return ''
  return String(marked.parse(src))
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

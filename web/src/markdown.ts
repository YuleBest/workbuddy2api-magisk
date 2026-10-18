// markdown.ts 助手回复的 markdown 渲染。
//
// html:false 是安全底线而不是风格选择：回复内容来自上游模型，属于不可信输入，而面板
// 的 admin token 就在 localStorage 里——一旦允许裸 HTML 经 v-html 注入，等于把管理
// token 交给任何能诱导模型输出 <img onerror=...> 的人。markdown-it 在 html:false 下
// 把裸 HTML 转义成字面文本，并默认拒绝 javascript: 之类的链接协议，因此不必再引
// DOMPurify。

import MarkdownIt from 'markdown-it'

const md = new MarkdownIt({ html: false, linkify: true, breaks: true })

// 链接一律新开标签：否则点一下就把面板整页导航走，当前对话直接丢掉。
const linkOpen = md.renderer.rules.link_open
md.renderer.rules.link_open = (tokens, idx, _options, _env, self) => {
  tokens[idx].attrSet('target', '_blank')
  tokens[idx].attrSet('rel', 'noopener noreferrer')
  return linkOpen
    ? linkOpen(tokens, idx, _options, _env, self)
    : self.renderToken(tokens, idx, _options)
}

export function renderMarkdown(text: string): string {
  return md.render(text)
}

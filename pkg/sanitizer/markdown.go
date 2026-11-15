// Package sanitizer 安全 Markdown 子集渲染（规格 8.3）。
// 支持子集：标题、粗体、斜体、行内代码、围栏代码块、无序/有序列表、链接、图片。
// 安全策略：先整体 HTML 转义再生成白名单标签——原始 HTML、脚本、iframe、javascript: URL 一律不可能输出；
// 链接与图片 URL 仅允许 http/https。禁止 SVG。
package sanitizer

import (
	"html"
	"regexp"
	"strings"
)

var (
	// 行内规则：作用于已转义文本；模式受限无回溯风险
	boldRe   = regexp.MustCompile(`\*\*([^*\n]{1,200})\*\*`)
	italicRe = regexp.MustCompile(`\*([^*\n]{1,200})\*`)
	codeRe   = regexp.MustCompile("`([^`\n]{1,200})`")
	linkRe   = regexp.MustCompile(`\[([^\]\n]{1,200})\]\((https?://[^)\s]{1,500})\)`)
	imgRe    = regexp.MustCompile(`!\[([^\]\n]{0,200})\]\((https?://[^)\s]{1,500}\.(?i:png|jpe?g|gif|webp))\)`)
)

// RenderMarkdown 渲染安全 HTML
func RenderMarkdown(src string) string {
	var out strings.Builder
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	inCode := false
	inUL := false
	inOL := false

	closeList := func() {
		if inUL {
			out.WriteString("</ul>\n")
			inUL = false
		}
		if inOL {
			out.WriteString("</ol>\n")
			inOL = false
		}
	}

	for _, line := range lines {
		// 围栏代码块
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inCode {
				out.WriteString("</code></pre>\n")
				inCode = false
			} else {
				closeList()
				out.WriteString("<pre><code>")
				inCode = true
			}
			continue
		}
		if inCode {
			// 代码块内容转义
			out.WriteString(html.EscapeString(line))
			out.WriteString("\n")
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			closeList()
			continue
		}

		// 标题（h2/h3/h4，避免与页面 h1 冲突）
		switch {
		case strings.HasPrefix(trimmed, "### "):
			closeList()
			out.WriteString("<h4>" + renderInline(html.EscapeString(trimmed[4:])) + "</h4>\n")
			continue
		case strings.HasPrefix(trimmed, "## "):
			closeList()
			out.WriteString("<h3>" + renderInline(html.EscapeString(trimmed[3:])) + "</h3>\n")
			continue
		case strings.HasPrefix(trimmed, "# "):
			closeList()
			out.WriteString("<h2>" + renderInline(html.EscapeString(trimmed[2:])) + "</h2>\n")
			continue
		}

		// 列表
		if strings.HasPrefix(trimmed, "- ") {
			if inOL {
				out.WriteString("</ol>\n")
				inOL = false
			}
			if !inUL {
				out.WriteString("<ul>\n")
				inUL = true
			}
			out.WriteString("<li>" + renderInline(html.EscapeString(trimmed[2:])) + "</li>\n")
			continue
		}
		if m := olRe.FindStringSubmatch(trimmed); m != nil {
			if inUL {
				out.WriteString("</ul>\n")
				inUL = false
			}
			if !inOL {
				out.WriteString("<ol>\n")
				inOL = true
			}
			out.WriteString("<li>" + renderInline(html.EscapeString(m[1])) + "</li>\n")
			continue
		}

		// 普通段落
		closeList()
		out.WriteString("<p>" + renderInline(html.EscapeString(trimmed)) + "</p>\n")
	}
	if inCode {
		out.WriteString("</code></pre>\n")
	}
	closeList()
	return out.String()
}

var olRe = regexp.MustCompile(`^\d{1,3}\. (.+)$`)

// renderInline 行内规则（输入已转义；URL 白名单 http/https）
func renderInline(escaped string) string {
	s := escaped
	// 图片（https 且扩展名白名单）
	s = imgRe.ReplaceAllString(s, `<img src="$2" alt="$1" loading="lazy">`)
	// 链接（http/https；rel 防钓鱼）
	s = linkRe.ReplaceAllString(s, `<a href="$2" rel="noopener noreferrer nofollow" target="_blank">$1</a>`)
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")
	s = italicRe.ReplaceAllString(s, "<em>$1</em>")
	s = codeRe.ReplaceAllString(s, "<code>$1</code>")
	return s
}

// StripMarkdown 提取纯文本摘要（用于搜索索引与 Feed 摘要）
func StripMarkdown(src string) string {
	var out strings.Builder
	inCode := false
	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			out.WriteString(t + "\n")
			continue
		}
		t = strings.TrimPrefix(t, "# ")
		t = strings.TrimPrefix(t, "## ")
		t = strings.TrimPrefix(t, "### ")
		t = strings.TrimPrefix(t, "- ")
		t = strings.TrimSpace(t)
		if t != "" {
			out.WriteString(t + "\n")
		}
	}
	return out.String()
}

package sanitizer

import (
	"strings"
	"testing"
)

// XSS 防护验收（规格 15.1）：脚本、事件属性、javascript: URL 一律不可输出
func TestSanitizerBlocksXSS(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		mustNot []string
	}{
		{
			name:    "raw script tag escaped",
			input:   "<script>alert(1)</script>",
			mustNot: []string{"<script"},
		},
		{
			name:    "inline event escaped",
			input:   "<img src=x onerror=alert(1)>",
			mustNot: []string{"<img src=x onerror"}, // 转义后的文本可以包含字样，但不能是活标签
		},
		{
			name:    "javascript url in link rejected",
			input:   "[click](javascript:alert(1))",
			// javascript: 不是 http(s) 白名单，不会生成 <a> 活链接；字面文本已转义
			mustNot: []string{"<a href=\"javascript:"},
		},
		{
			name:    "iframe escaped",
			input:   "<iframe src='https://evil.example'></iframe>",
			mustNot: []string{"<iframe"},
		},
		{
			name:    "svg not allowed as image",
			input:   "![x](https://evil.example/a.svg)",
			mustNot: []string{"<img"},
		},
		{
			name:    "data url image rejected",
			input:   "![x](data:text/html;base64,PHNjcmlwdD4=)",
			mustNot: []string{"<img src=\"data:"}, // data: URL 不进白名单，无 <img> 活标签
		},
	}
	for _, tc := range cases {
		out := RenderMarkdown(tc.input)
		for _, banned := range tc.mustNot {
			if strings.Contains(out, banned) {
				t.Fatalf("%s: output contains %q\ninput: %q\noutput: %q", tc.name, banned, tc.input, out)
			}
		}
	}
}

// 白名单功能仍可用
func TestSanitizerAllowsSubset(t *testing.T) {
	out := RenderMarkdown("# 标题\n\n**粗体** 与 *斜体* 与 `code`\n\n- 甲\n- 乙\n\n[链接](https://example.com/a)\n\n![图](https://cdn.example.com/a.png)\n\n```go\ncode block\n```")
	for _, want := range []string{"<h2>标题</h2>", "<strong>粗体</strong>", "<em>斜体</em>", "<code>code</code>", "<li>甲</li>", `href="https://example.com/a"`, `<img src="https://cdn.example.com/a.png"`, "<pre><code>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
	if !strings.Contains(out, `rel="noopener noreferrer nofollow"`) {
		t.Fatal("link must carry rel attributes")
	}
}

// StripMarkdown 摘要提取
func TestStripMarkdown(t *testing.T) {
	got := StripMarkdown("# 标题\n\n- 项目一\n正文内容")
	if !strings.Contains(got, "标题") || !strings.Contains(got, "正文内容") {
		t.Fatalf("strip failed: %q", got)
	}
}

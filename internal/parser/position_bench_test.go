package parser

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// benchPage builds a page shaped like a typical template-driven site: a masthead,
// a menu, an article with in-body links, a sidebar module and a large footer.
func benchPage() string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><body class="page-id-1274 single">`)
	b.WriteString(`<header id="masthead" class="site-header"><a href="/">Home</a>`)
	b.WriteString(`<nav class="primary-menu"><ul>`)
	for i := 0; i < 25; i++ {
		fmt.Fprintf(&b, `<li class="menu-item"><a href="/section/%d">Section %d</a></li>`, i, i)
	}
	b.WriteString(`</ul></nav></header>`)
	b.WriteString(`<main id="content"><article class="post"><div class="entry-content">`)
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&b, `<p>Some body copy around <a href="/article/%d">an inline link</a> in a sentence.</p>`, i)
	}
	b.WriteString(`</div></article></main>`)
	b.WriteString(`<aside class="sidebar"><div class="widget related-items"><ul>`)
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, `<li><a href="/related/%d">Related %d</a></li>`, i, i)
	}
	b.WriteString(`</ul></div></aside>`)
	b.WriteString(`<footer class="site-footer"><div class="footer-columns"><ul>`)
	for i := 0; i < 45; i++ {
		fmt.Fprintf(&b, `<li><a href="/footer/%d">Footer %d</a></li>`, i, i)
	}
	b.WriteString(`</ul></div></footer></body></html>`)
	return b.String()
}

func benchmarkExtractLinks(b *testing.B, opts Options) {
	page := benchPage()
	base, err := url.Parse("https://example.com/page")
	if err != nil {
		b.Fatalf("url.Parse() error = %v", err)
	}
	doc := docFromHTML(page)

	links := extractLinks(doc, base, opts)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractLinks(doc, base, opts)
	}
	b.StopTimer()

	// The retained cost of the feature is the XPath string kept per link;
	// everything else it records is fixed-size.
	xpathBytes := 0
	for _, l := range links {
		xpathBytes += len(l.XPath)
	}
	b.ReportMetric(float64(xpathBytes)/float64(len(links)), "xpath-B/link")
}

func BenchmarkExtractLinks(b *testing.B) {
	benchmarkExtractLinks(b, Options{LinkPosition: false})
}

func BenchmarkExtractLinksWithPosition(b *testing.B) {
	benchmarkExtractLinks(b, Options{LinkPosition: true})
}

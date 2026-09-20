package scrape

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// minimalPDF builds a one-page PDF whose content stream shows text, with a
// correct xref table, so the parser finds a real text layer.
func minimalPDF(text string) []byte {
	content := fmt.Sprintf("BT /F1 18 Tf 72 700 Td (%s) Tj ET", text)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, obj := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return []byte(b.String())
}

func TestPDFToTextAndFetchPage(t *testing.T) {
	doc := minimalPDF("Attention is all you need")
	text, err := PDFToText(doc)
	if err != nil || !strings.Contains(text, "Attention is all you need") {
		t.Fatalf("PDFToText = %q, %v", text, err)
	}
	if !IsPDF("application/pdf", nil) || !IsPDF("application/octet-stream", doc) || IsPDF("text/html", []byte("<html>")) {
		t.Error("IsPDF detection wrong")
	}
	if _, err := PDFToText([]byte("%PDF-1.4 garbage")); err == nil {
		t.Error("malformed PDF should error, not panic")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/octet" {
			w.Header().Set("Content-Type", "application/octet-stream")
		} else {
			w.Header().Set("Content-Type", "application/pdf")
		}
		_, _ = w.Write(doc)
	}))
	t.Cleanup(srv.Close)
	f := &Fetcher{}
	got, isPDF, err := f.FetchPage(context.Background(), srv.URL+"/paper.pdf")
	if err != nil || !isPDF || !strings.Contains(got, "Attention is all you need") {
		t.Errorf("FetchPage = %q, pdf=%v, %v", got, isPDF, err)
	}
	if _, isPDF, err := f.FetchPage(context.Background(), srv.URL+"/octet"); err != nil || !isPDF {
		t.Errorf("octet-stream PDF: pdf=%v, %v", isPDF, err)
	}
	pages := f.FetchAll(context.Background(), []string{srv.URL + "/a.pdf"}, 1)
	if !pages[0].PDF || pages[0].Err != nil {
		t.Errorf("FetchAll page = %+v", pages[0])
	}
}

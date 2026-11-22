package transcript

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
)

func TestCSVPreservesUnicodeAndNeutralizesFormulas(t *testing.T) {
	entries := []models.TranscriptEntry{
		{CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("fixture", 3600)), Content: "Привет, \"мир\"\nSecond line"},
		{Content: "  =1+1"},
		{Content: "\tplain"},
		{Content: "\ufeff@formula"},
		{Content: "＝1+1"},
	}
	data, err := CSV(entries)
	if err != nil || !bytes.HasPrefix(data, []byte("\ufeff")) {
		t.Fatalf("CSV export failed: %v", err)
	}
	rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte("\ufeff")))).ReadAll()
	if err != nil || len(rows) != 6 || rows[0][6] != "content" || rows[1][0] != "2026-01-02T02:04:05Z" {
		t.Fatalf("unreadable CSV or unexpected timestamp: %v", err)
	}
	for i, want := range []string{"Привет, \"мир\"\nSecond line", "'  =1+1", "'\tplain", "'\ufeff@formula", "'＝1+1"} {
		if rows[i+1][6] != want {
			t.Fatalf("unexpected spreadsheet cell on row %d: %q", i+1, rows[i+1][6])
		}
	}
}

func TestPDFUsesCyrillicFontAndPaginates(t *testing.T) {
	font, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	var buffer sfnt.Buffer
	if got := pdfText("Привет\r\nМир 😀", font, &buffer); got != "Привет\nМир [U+1F600]" {
		t.Fatalf("unexpected plain text: %q", got)
	}
	content := strings.Repeat("Тестовая строка с кириллицей.\n", 100)
	data, err := PDF([]models.TranscriptEntry{{Content: content, SentByOperator: true, ImpersonatedRole: "client"}})
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF-")) ||
		!bytes.Contains(data, []byte("/ToUnicode")) || !bytes.Contains(data, []byte("/Count 2")) {
		t.Fatalf("unreadable or unpaginated PDF: %v", err)
	}
	if _, err := PDF([]models.TranscriptEntry{{Content: strings.Repeat("\n", 20000)}}); !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("oversized PDF returned %v", err)
	}
}

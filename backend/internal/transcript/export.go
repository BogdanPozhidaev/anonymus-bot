package transcript

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"codeberg.org/go-pdf/fpdf"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
)

const maxPDFPages = 250

var ErrTooManyPages = errors.New("transcript exceeds PDF page limit")

func CSV(entries []models.TranscriptEntry) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("\ufeff")
	writer := csv.NewWriter(&output)
	records := make([][]string, 0, len(entries)+1)
	records = append(records, []string{"timestamp_utc", "sender_role", "content_type", "delivery_status", "sent_by_operator", "impersonated_role", "content"})
	for _, entry := range entries {
		records = append(records, []string{
			entry.CreatedAt.UTC().Format(time.RFC3339Nano), entry.SenderRole, entry.ContentType,
			entry.DeliveryStatus, strconv.FormatBool(entry.SentByOperator), entry.ImpersonatedRole, safeCSVText(entry.Content),
		})
	}
	if err := writer.WriteAll(records); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func safeCSVText(value string) string {
	leading := strings.TrimLeftFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || r == '\ufeff'
	})
	first, _ := utf8.DecodeRuneInString(value)
	sign, _ := utf8.DecodeRuneInString(leading)
	if strings.ContainsRune("\t\r\n", first) || strings.ContainsRune("=+-@＝＋－＠", sign) {
		return "'" + value
	}
	return value
}

func PDF(entries []models.TranscriptEntry) ([]byte, error) {
	font, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddUTF8FontFromBytes("Go", "", goregular.TTF)
	pdf.SetTitle("Session transcript", false)
	pdf.AddPage()
	pdf.SetFont("Go", "", 16)
	pdf.CellFormat(180, 10, "Session transcript", "", 1, "L", false, 0, "")
	pdf.SetFont("Go", "", 10)

	var glyphBuffer sfnt.Buffer
	for _, entry := range entries {
		label := fmt.Sprintf("%s  |  %s  |  %s  |  %s",
			entry.CreatedAt.UTC().Format(time.RFC3339Nano), entry.SenderRole, entry.ContentType, entry.DeliveryStatus)
		if entry.SentByOperator {
			label += "  |  Operator"
			if entry.ImpersonatedRole != "" {
				label += " as " + entry.ImpersonatedRole
			}
		}
		pdf.SetTextColor(45, 63, 82)
		if err := writePDFLines(pdf, label); err != nil {
			return nil, err
		}
		pdf.SetTextColor(22, 28, 36)
		if entry.Content != "" {
			text := pdfText(entry.Content, font, &glyphBuffer)
			if err := writePDFLines(pdf, text); err != nil {
				return nil, err
			}
		}
		pdf.Ln(2)
	}
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

var pdfWhitespace = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", "    ")

func pdfText(text string, font *sfnt.Font, glyphBuffer *sfnt.Buffer) string {
	text = pdfWhitespace.Replace(text)
	var output strings.Builder
	output.Grow(len(text))
	for _, r := range text {
		index, err := font.GlyphIndex(glyphBuffer, r)
		if r == '\n' || (r <= 0xffff && !unicode.IsControl(r) && index != 0 && err == nil) {
			output.WriteRune(r)
		} else {
			_, _ = fmt.Fprintf(&output, "[U+%04X]", r)
		}
	}
	return output.String()
}

func writePDFLines(pdf *fpdf.Fpdf, text string) error {
	const width = 180
	for paragraph := range strings.SplitSeq(text, "\n") {
		lines := pdf.SplitText(paragraph, width)
		if len(lines) == 0 {
			lines = []string{""}
		}
		for _, line := range lines {
			pdf.CellFormat(width, 5, line, "", 1, "L", false, 0, "")
			if pdf.PageNo() > maxPDFPages {
				return ErrTooManyPages
			}
		}
	}
	return pdf.Error()
}

package moderation

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestLabelledCorpus(t *testing.T) {
	f, err := os.Open("testdata/corpus.tsv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })

	var truePositive, trueNegative, falsePositive, falseNegative int
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		label, sample, ok := strings.Cut(scan.Text(), "\t")
		if !ok || sample == "" || (label != "allowed" && label != "contact") {
			t.Fatal("malformed corpus row")
		}
		matched, _ := ContainsContactInfo(sample)
		suspicious, _ := ContainsSuspiciousPhrase(sample)
		switch detected := matched || suspicious; {
		case detected && label == "contact":
			truePositive++
		case detected:
			falsePositive++
		case label == "allowed":
			trueNegative++
		default:
			falseNegative++
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
	if total := truePositive + trueNegative + falsePositive + falseNegative; total < 150 {
		t.Fatalf("only %d labelled corpus rows", total)
	}
	precision := float64(truePositive) / float64(truePositive+falsePositive)
	recall := float64(truePositive) / float64(truePositive+falseNegative)
	t.Logf("TP=%d TN=%d FP=%d FN=%d precision=%.2f%% recall=%.2f%%", truePositive, trueNegative, falsePositive, falseNegative, precision*100, recall*100)
	if !(precision >= 0.90 && recall >= 0.90) {
		t.Fatal("moderation quality fell below the corpus thresholds")
	}
}

func TestModerationConfiguration(t *testing.T) {
	cfg, err := ParseConfig("email, phone", "true")
	if err != nil || !cfg.autoPause || !cfg.disabled["email"] || !cfg.disabled["phone"] || cfg.disabled["username"] {
		t.Fatalf("unexpected configured rules: %+v, %v", cfg, err)
	}
	if found, _ := matchContact("fixture@example.invalid", cfg.disabled); found {
		t.Fatal("explicitly disabled email rule applied")
	}
	if found, _ := matchContact("@fixture_quartz", cfg.disabled); !found {
		t.Fatal("unrelated rule was disabled")
	}
	for _, args := range [][2]string{{"unrecognized", ""}, {"", "yes"}} {
		if _, err := ParseConfig(args[0], args[1]); err == nil {
			t.Fatal("invalid moderation configuration was accepted")
		}
	}
}

package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadDir(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "01-基础规范"), 0755)
	os.MkdirAll(filepath.Join(dir, "02-素材库"), 0755)

	os.WriteFile(filepath.Join(dir, "01-基础规范", "01-写作总则.md"), []byte("# 写作总则\n内容第一"), 0644)
	os.WriteFile(filepath.Join(dir, "01-基础规范", "02-禁忌.md"), []byte("# 禁忌\n不说脏话"), 0644)
	os.WriteFile(filepath.Join(dir, "02-素材库", "金句库.md"), []byte("# 金句\n学无止境"), 0644)

	result, err := readLocalDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if result == "" {
		t.Error("empty result")
	}
	t.Logf("Prompt:\n%s", result)
}

func TestReadDir_Empty(t *testing.T) {
	dir := t.TempDir()
	_, err := readLocalDir(dir)
	if err == nil {
		t.Error("expected error for empty dir")
	}
}

package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	b, e := os.ReadFile("testdata/synthetic-electronics.xlsx")
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func altered(t *testing.T, original []byte, edit func(string, string) (string, string)) []byte {
	t.Helper()
	z, e := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, f := range z.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		_ = r.Close()
		name, data := edit(f.Name, string(b))
		p, e := w.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		_, _ = p.Write([]byte(data))
	}
	_ = w.Close()
	return out.Bytes()
}
func TestXLSXOfflineSecurity(t *testing.T) {
	b := fixture(t)
	g, e := ReadXLSX(context.Background(), b)
	if e != nil || len(g) != 7 || len(g[0]) != 11 {
		t.Fatal("valid synthetic", e)
	}
	for _, mode := range []string{"formula", "macro", "external", "path", "doctype", "broken_xml", "zip_bomb", "columns", "rows", "corrupt", "empty", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			bad := altered(t, b, func(name, data string) (string, string) {
				if name == "xl/worksheets/sheet1.xml" {
					switch mode {
					case "formula":
						data = strings.Replace(data, "<is>", "<f>1+1</f><is>", 1)
					case "doctype":
						data = "<!DOCTYPE x [<!ENTITY a SYSTEM 'file:///etc/passwd'>]>" + data
					case "broken_xml":
						data += "<bad>"
					case "zip_bomb":
						data = strings.Repeat("x", 5<<20)
					case "columns":
						data = strings.Replace(data, "A1", "AZ1", 1)
					case "rows":
						data = strings.Replace(data, `r="7"`, `r="900000"`, 1)
					case "path":
						name = "../evil.xml"
					}
				}
				if mode == "macro" && name == "xl/workbook.xml" {
					name = "xl/vbaProject.bin"
				}
				if mode == "external" && strings.HasSuffix(name, ".rels") {
					data = strings.Replace(data, "<Relationship ", `<Relationship TargetMode="External" `, 1)
				}
				return name, data
			})
			switch mode {
			case "corrupt":
				bad = []byte("PK\x03\x04broken")
			case "empty":
				bad = nil
			case "oversize":
				bad = make([]byte, MaxXLSXBytes+1)
			}
			if _, e := ReadXLSX(context.Background(), bad); e == nil {
				t.Fatal("unsafe workbook accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := ReadXLSX(ctx, b); e == nil {
		t.Fatal("cancelled parser")
	}
}
func TestSharedStringExpansionBound(t *testing.T) {
	b := fixture(t)
	z, _ := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	sheet := `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1">`
	for col := 0; col < 11; col++ {
		sheet += fmt.Sprintf(`<c r="%c1" t="inlineStr"><is><t>header%d</t></is></c>`, 65+col, col)
	}
	sheet += `</row>`
	for row := 2; row <= 100; row++ {
		sheet += fmt.Sprintf(`<row r="%d">`, row)
		for col := 0; col < 11; col++ {
			sheet += fmt.Sprintf(`<c r="%c%d" t="s"><v>0</v></c>`, 65+col, row)
		}
		sheet += `</row>`
	}
	sheet += `</sheetData></worksheet>`
	for _, f := range z.File {
		r, _ := f.Open()
		data, _ := io.ReadAll(r)
		_ = r.Close()
		if f.Name == "xl/worksheets/sheet1.xml" {
			data = []byte(sheet)
		}
		entry, _ := w.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Store})
		_, _ = entry.Write(data)
	}
	entry, _ := w.CreateHeader(&zip.FileHeader{Name: "xl/sharedStrings.xml", Method: zip.Store})
	_, _ = entry.Write([]byte(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>` + strings.Repeat("a", 16000) + `</t></si></sst>`))
	_ = w.Close()
	if _, e := ReadXLSX(context.Background(), out.Bytes()); e == nil {
		t.Fatal("shared-string expansion unbounded")
	}
}

func TestImportRowValidation(t *testing.T) {
	for _, test := range []struct {
		sku, price, quantity string
		reason               string
	}{{"OK", "-1", "1", ""}, {"OK", "1.999", "1", ""}, {"OK", "1", "-1", ""}, {"OK", "1", "1000000001", ""}, {"bad space", "1", "1", ""}} {
		lines, e := normalize(Grid{{"sku", "name", "price", "quantity"}, {test.sku, "Synthetic", test.price, test.quantity}}, map[string]int{"sku": 0, "name": 1, "price": 2, "quantity": 3})
		if e != nil || lines[0].State != "ERROR" {
			t.Fatal("bad row accepted", test, e)
		}
	}
	lines, e := normalize(Grid{{"sku", "name", "price", "quantity"}, {"dupe", "x", "1,25", "0"}, {" DUPE ", "x", "1", "0"}}, map[string]int{"sku": 0, "name": 1, "price": 2, "quantity": 3})
	if e != nil || lines[0].State != "DUPLICATE" || lines[1].State != "DUPLICATE" || lines[0].Input.PriceAmount != 125 {
		t.Fatal("duplicates/money", e)
	}
	badAttrs, e := normalize(Grid{{"sku", "name", "price", "quantity", "model", "attributes"}, {"NULL-ATTRS", "Synthetic", "1", "1", "test", "null"}}, map[string]int{"sku": 0, "name": 1, "price": 2, "quantity": 3, "model": 4, "attributes": 5})
	if e != nil || badAttrs[0].State != "ERROR" {
		t.Fatal("null attributes not rejected safely", e)
	}
	for _, m := range []map[string]int{{"sku": 0}, {"sku": 0, "name": 0, "price": 2, "quantity": 3}, {"sku": 100, "name": 1, "price": 2, "quantity": 3}, {"sku": 0, "name": 1, "price": 2, "quantity": 3, "tenant_id": 0}} {
		if _, e := normalize(Grid{{"a", "b", "c", "d"}, {"a", "b", "1", "1"}}, m); e == nil {
			t.Fatal("invalid mapping accepted")
		}
	}
}

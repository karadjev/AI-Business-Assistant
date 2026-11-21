package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxXLSXBytes = 1 << 20
const MaxImportRows = 500
const MaxImportColumns = 24

var ErrWorkbook = errors.New("XLSX отклонён: нужен один обычный лист, без формул, макросов и внешних ссылок; максимум 1 MiB, 500 строк, 24 колонки")

type Grid [][]string

// ReadXLSX never extracts files, evaluates formulae, opens URLs, or uses external entities.
func ReadXLSX(ctx context.Context, b []byte) (Grid, error) {
	if len(b) < 4 || len(b) > MaxXLSXBytes || !bytes.Equal(b[:4], []byte{'P', 'K', 3, 4}) {
		return nil, ErrWorkbook
	}
	z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if e != nil || len(z.File) > 64 {
		return nil, ErrWorkbook
	}
	parts := map[string][]byte{}
	names := map[string]bool{}
	var total uint64
	for _, f := range z.File {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		n := strings.TrimSuffix(f.Name, "/")
		lower := strings.ToLower(n)
		if n == "" || path.Clean(n) != n || strings.HasPrefix(n, "/") || strings.HasPrefix(n, "../") || strings.ContainsAny(n, "\\\x00") || names[lower] || f.Flags&1 != 0 {
			return nil, ErrWorkbook
		}
		names[lower] = true
		if f.FileInfo().IsDir() {
			continue
		}
		if !f.Mode().IsRegular() {
			return nil, ErrWorkbook
		}
		for _, bad := range []string{"vbaproject", "macro", "externallink", "embedding", "activex", "connections", ".bin"} {
			if strings.Contains(lower, bad) {
				return nil, ErrWorkbook
			}
		}
		total += f.UncompressedSize64
		if total > 8<<20 || f.UncompressedSize64 > 4<<20 || f.UncompressedSize64 > (f.CompressedSize64+1)*100 {
			return nil, ErrWorkbook
		}
		if !strings.HasSuffix(n, ".xml") && !strings.HasSuffix(n, ".rels") {
			return nil, ErrWorkbook
		}
		r, e := f.Open()
		if e != nil {
			return nil, ErrWorkbook
		}
		raw, e := io.ReadAll(io.LimitReader(r, (4<<20)+1))
		_ = r.Close()
		if e != nil || len(raw) > 4<<20 {
			return nil, ErrWorkbook
		}
		if !utf8.Valid(raw) || bytes.Contains(bytes.ToUpper(raw), []byte("<!DOCTYPE")) {
			return nil, ErrWorkbook
		}
		// Parse every XML part; do not hide malformed/active parts in an unused entry.
		dec := xml.NewDecoder(bytes.NewReader(raw))
		for {
			token, e := dec.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, ErrWorkbook
			}
			if s, ok := token.(xml.StartElement); ok {
				for _, a := range s.Attr {
					if (s.Name.Local == "Relationship" && a.Name.Local == "TargetMode" && strings.EqualFold(a.Value, "External")) || strings.Contains(strings.ToLower(a.Value), "macroenabled") {
						return nil, ErrWorkbook
					}
				}
			}
		}
		parts[n] = raw
	}
	if len(parts["[Content_Types].xml"]) == 0 || len(parts["_rels/.rels"]) == 0 {
		return nil, ErrWorkbook
	}
	var ct struct {
		Overrides []struct {
			PartName    string `xml:",attr"`
			ContentType string `xml:",attr"`
		} `xml:"Override"`
	}
	if xml.Unmarshal(parts["[Content_Types].xml"], &ct) != nil {
		return nil, ErrWorkbook
	}
	main := false
	for _, o := range ct.Overrides {
		if o.PartName == "/xl/workbook.xml" && o.ContentType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml" {
			main = true
		}
	}
	if !main {
		return nil, ErrWorkbook
	}
	var wb struct {
		Sheets []struct {
			Name  string `xml:"name,attr"`
			ID    string `xml:"id,attr"`
			State string `xml:"state,attr"`
		} `xml:"sheets>sheet"`
	}
	if xml.Unmarshal(parts["xl/workbook.xml"], &wb) != nil || len(wb.Sheets) != 1 || wb.Sheets[0].State != "" && wb.Sheets[0].State != "visible" {
		return nil, ErrWorkbook
	}
	var rels struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
			Type   string `xml:"Type,attr"`
		} `xml:"Relationship"`
	}
	if xml.Unmarshal(parts["xl/_rels/workbook.xml.rels"], &rels) != nil {
		return nil, ErrWorkbook
	}
	sheet := ""
	for _, r := range rels.Items {
		if r.ID == wb.Sheets[0].ID && strings.HasSuffix(r.Type, "/worksheet") {
			if strings.Contains(r.Target, "..") || strings.ContainsAny(r.Target, "\\:") {
				return nil, ErrWorkbook
			}
			sheet = path.Join("xl", r.Target)
			if strings.HasPrefix(r.Target, "/") {
				sheet = strings.TrimPrefix(r.Target, "/")
			}
		}
	}
	if sheet == "" || len(parts[sheet]) == 0 {
		return nil, ErrWorkbook
	}
	type rich struct {
		T    string `xml:"t"`
		Runs []struct {
			T string `xml:"t"`
		} `xml:"r"`
	}
	getText := func(r rich) string {
		s := r.T
		for _, v := range r.Runs {
			s += v.T
		}
		return s
	}
	var ss struct {
		Items []rich `xml:"si"`
	}
	if raw := parts["xl/sharedStrings.xml"]; raw != nil {
		if xml.Unmarshal(raw, &ss) != nil || len(ss.Items) > 12024 {
			return nil, ErrWorkbook
		}
	}
	var ws struct {
		Rows []struct {
			Number int    `xml:"r,attr"`
			Hidden string `xml:"hidden,attr"`
			Cells  []struct {
				Ref     string    `xml:"r,attr"`
				Type    string    `xml:"t,attr"`
				Formula *struct{} `xml:"f"`
				Value   string    `xml:"v"`
				Inline  rich      `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
		Merges []struct{} `xml:"mergeCells>mergeCell"`
		Cols   []struct {
			Hidden string `xml:"hidden,attr"`
		} `xml:"cols>col"`
	}
	if xml.Unmarshal(parts[sheet], &ws) != nil || len(ws.Rows) < 2 || len(ws.Rows) > MaxImportRows+1 || len(ws.Merges) > 0 {
		return nil, ErrWorkbook
	}
	for _, c := range ws.Cols {
		if c.Hidden == "1" || c.Hidden == "true" {
			return nil, ErrWorkbook
		}
	}
	expandedText := 0
	grid := Grid{}
	width := 0
	prev := 0
	for _, r := range ws.Rows {
		if r.Number <= prev || r.Number > MaxImportRows+1 || r.Hidden == "1" || r.Hidden == "true" || len(r.Cells) > MaxImportColumns {
			return nil, ErrWorkbook
		}
		prev = r.Number
		for len(grid) < r.Number-1 {
			grid = append(grid, make([]string, MaxImportColumns))
		}
		row := make([]string, MaxImportColumns)
		seen := map[int]bool{}
		for _, c := range r.Cells {
			if c.Formula != nil {
				return nil, ErrWorkbook
			}
			i := 0
			col := 0
			for i < len(c.Ref) && c.Ref[i] >= 'A' && c.Ref[i] <= 'Z' {
				col = col*26 + int(c.Ref[i]-'A'+1)
				if col > MaxImportColumns {
					return nil, ErrWorkbook
				}
				i++
			}
			rn, e := strconv.Atoi(c.Ref[i:])
			if e != nil || rn != r.Number || col < 1 || col > MaxImportColumns || seen[col] {
				return nil, ErrWorkbook
			}
			seen[col] = true
			v := c.Value
			switch c.Type {
			case "s":
				n, e := strconv.Atoi(v)
				if e != nil || n < 0 || n >= len(ss.Items) {
					return nil, ErrWorkbook
				}
				v = getText(ss.Items[n])
			case "inlineStr":
				v = getText(c.Inline)
			case "", "n", "str", "b":
			default:
				return nil, ErrWorkbook
			}
			expandedText += len(v)
			if expandedText > 1<<20 {
				return nil, ErrWorkbook
			}
			if len(v) > 16384 || strings.ContainsRune(v, 0) || strings.HasPrefix(strings.TrimSpace(v), "=") {
				return nil, ErrWorkbook
			}
			row[col-1] = strings.TrimSpace(v)
			if col > width {
				width = col
			}
		}
		grid = append(grid, row)
	}
	if width == 0 {
		return nil, ErrWorkbook
	}
	for i := range grid {
		grid[i] = grid[i][:width]
	}
	for _, h := range grid[0] {
		if h == "" || len(h) > 120 {
			return nil, ErrWorkbook
		}
	}
	raw, _ := json.Marshal(grid)
	if len(raw) > 2<<20 {
		return nil, ErrWorkbook
	}
	return grid, nil
}

// CatalogFeed is the future adapter boundary. No network/1C implementation is installed.
// All future adapters must use the same validation, preview, source policy and transaction.
type CatalogFeed interface {
	Source() string
	Read(context.Context, io.Reader) ([]Input, error)
}

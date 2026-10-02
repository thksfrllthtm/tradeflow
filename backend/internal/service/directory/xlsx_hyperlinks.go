package directory

// Hyperlinks of one worksheet read straight from the XLSX package. excelize only looks
// links up cell by cell, scanning every link each time — too slow for 13 000 rows.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path"
	"strings"
)

type xlsxRelationships struct {
	Relationships []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

// sheetHyperlinks maps cell references («D2») to link targets for the sheet named sheet.
func sheetHyperlinks(data []byte, sheet string) (map[string]string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := map[string]*zip.File{}
	for _, file := range archive.File {
		files[strings.TrimPrefix(file.Name, "/")] = file
	}
	read := func(name string) ([]byte, error) {
		file, ok := files[name]
		if !ok {
			return nil, nil
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(io.LimitReader(reader, 64<<20))
	}
	relations := func(name string) (map[string]string, error) {
		raw, err := read(name)
		if err != nil || raw == nil {
			return map[string]string{}, err
		}
		var parsed xlsxRelationships
		if err := xml.Unmarshal(raw, &parsed); err != nil {
			return nil, err
		}
		byID := make(map[string]string, len(parsed.Relationships))
		for _, relation := range parsed.Relationships {
			byID[relation.ID] = relation.Target
		}
		return byID, nil
	}

	// xl/workbook.xml: sheet name → relationship id → worksheet part.
	raw, err := read("xl/workbook.xml")
	if err != nil || raw == nil {
		return map[string]string{}, err
	}
	var workbook struct {
		Sheets []struct {
			Name string     `xml:"name,attr"`
			Attr []xml.Attr `xml:",any,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := xml.Unmarshal(raw, &workbook); err != nil {
		return nil, err
	}
	sheetRelation := ""
	for _, item := range workbook.Sheets {
		if item.Name != sheet {
			continue
		}
		for _, attr := range item.Attr {
			if attr.Name.Local == "id" {
				sheetRelation = attr.Value
			}
		}
	}
	workbookRelations, err := relations("xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, err
	}
	target := workbookRelations[sheetRelation]
	if target == "" {
		return map[string]string{}, nil
	}
	sheetPath := strings.TrimPrefix(target, "/")
	if !strings.HasPrefix(target, "/") {
		sheetPath = path.Join("xl", target)
	}
	linkTargets, err := relations(path.Join(path.Dir(sheetPath), "_rels", path.Base(sheetPath)+".rels"))
	if err != nil {
		return nil, err
	}

	// <hyperlink ref="D2" r:id="rId1"/> — streamed: the sheet itself holds every row.
	file, ok := files[sheetPath]
	if !ok {
		return map[string]string{}, nil
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	links := map[string]string{}
	decoder := xml.NewDecoder(reader)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return links, nil
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "hyperlink" {
			continue
		}
		ref, relation := "", ""
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "ref":
				ref = attr.Value
			case "id":
				relation = attr.Value
			}
		}
		if ref != "" && linkTargets[relation] != "" {
			links[ref] = linkTargets[relation]
		}
	}
}

package fontglyph

import "encoding/binary"

type sfntTable struct {
	Tag    string
	Offset uint32
	Length uint32
}

func listSFNTTables(fontData []byte) ([]sfntTable, error) {
	if len(fontData) < 12 {
		return nil, ErrInvalidFontData
	}
	numTables := int(binary.BigEndian.Uint16(fontData[4:6]))
	dirEnd := 12 + numTables*16
	if numTables < 0 || dirEnd > len(fontData) {
		return nil, ErrInvalidFontData
	}
	tables := make([]sfntTable, 0, numTables)
	for offset := 12; offset < dirEnd; offset += 16 {
		tableOffset := binary.BigEndian.Uint32(fontData[offset+8 : offset+12])
		tableLength := binary.BigEndian.Uint32(fontData[offset+12 : offset+16])
		if uint64(tableOffset)+uint64(tableLength) > uint64(len(fontData)) {
			return nil, ErrInvalidFontData
		}
		tables = append(tables, sfntTable{
			Tag:    string(fontData[offset : offset+4]),
			Offset: tableOffset,
			Length: tableLength,
		})
	}
	return tables, nil
}

func getSFNTTable(fontData []byte, tag string) ([]byte, bool, error) {
	tables, err := listSFNTTables(fontData)
	if err != nil {
		return nil, false, err
	}
	for _, table := range tables {
		if table.Tag == tag {
			return fontData[table.Offset : table.Offset+table.Length], true, nil
		}
	}
	return nil, false, nil
}
